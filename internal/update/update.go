// Package update defines the shared transactional update contract for native and Compose deployments.
// It provides state management, readiness gates, rollback, and recovery status tracking.
package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State represents the current state of an update transaction.
type State string

const (
	// StateIdle means no update is in progress.
	StateIdle State = "idle"
	// StateDownloading means the release artifact is being downloaded.
	StateDownloading State = "downloading"
	// StateVerifying means the download checksum is being verified.
	StateVerifying State = "verifying"
	// StateInstalling means the new version is being installed.
	StateInstalling State = "installing"
	// StateReadiness means the system is waiting for readiness after installation.
	StateReadiness State = "readiness"
	// StateCompleted means the update finished successfully.
	StateCompleted State = "completed"
	// StateRolledBack means the update failed and was rolled back.
	StateRolledBack State = "rolled_back"
	// StateFailed means the update failed and rollback also failed.
	StateFailed State = "failed"
)

// Entry represents a single update attempt in the history.
type Entry struct {
	ID             string    `json:"id"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	FromVersion    string    `json:"from_version"`
	ToVersion      string    `json:"to_version"`
	State          State     `json:"state"`
	Error          string    `json:"error,omitempty"`
	ReadinessOK    bool      `json:"readiness_ok"`
	RollbackUsed   bool      `json:"rollback_used"`
	DeploymentMode string    `json:"deployment_mode"`
}

// Config defines the update contract configuration.
type Config struct {
	// ReadinessTimeout is the maximum time to wait for readiness after installation.
	ReadinessTimeout time.Duration `json:"readiness_timeout"`
	// ReadinessInterval is the interval between readiness checks.
	ReadinessInterval time.Duration `json:"readiness_interval"`
	// MaxHistory is the maximum number of update entries to retain.
	MaxHistory int `json:"max_history"`
	// StateFile is the path to persist update state across restarts.
	StateFile string `json:"state_file"`
}

// DefaultConfig returns sensible defaults for the update contract.
func DefaultConfig() Config {
	return Config{
		ReadinessTimeout:  5 * time.Minute,
		ReadinessInterval: 5 * time.Second,
		MaxHistory:        20,
		StateFile:         "/var/lib/velora/update-state.json",
	}
}

// Manager manages update transactions with durable state, readiness gates, and rollback.
type Manager struct {
	mu      sync.RWMutex
	config  Config
	current *Entry
	history []Entry
}

// NewManager creates a new update manager with the given configuration.
func NewManager(config Config) *Manager {
	if config.ReadinessTimeout == 0 {
		config.ReadinessTimeout = 5 * time.Minute
	}
	if config.ReadinessInterval == 0 {
		config.ReadinessInterval = 5 * time.Second
	}
	if config.MaxHistory == 0 {
		config.MaxHistory = 20
	}
	return &Manager{config: config}
}

// LoadState loads durable state from the state file if it exists.
func (m *Manager) LoadState() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.StateFile == "" {
		return nil
	}

	data, err := os.ReadFile(m.config.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read update state: %w", err)
	}

	var persisted struct {
		Current *Entry  `json:"current"`
		History []Entry `json:"history"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("decode update state: %w", err)
	}

	m.current = persisted.Current
	m.history = persisted.History
	return nil
}

// SaveState persists the current state to disk.
func (m *Manager) SaveState() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config.StateFile == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(m.config.StateFile), 0750); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	persisted := struct {
		Current *Entry  `json:"current"`
		History []Entry `json:"history"`
	}{
		Current: m.current,
		History: m.history,
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode update state: %w", err)
	}

	if err := os.WriteFile(m.config.StateFile, data, 0640); err != nil {
		return fmt.Errorf("write update state: %w", err)
	}

	return nil
}

// Begin starts a new update transaction. It rejects concurrent updates.
func (m *Manager) Begin(fromVersion, toVersion, deploymentMode string) (*Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil && m.current.State != StateCompleted && m.current.State != StateRolledBack && m.current.State != StateFailed {
		return nil, errors.New("update already in progress")
	}

	entry := &Entry{
		ID:             fmt.Sprintf("update-%d", time.Now().UnixNano()),
		StartedAt:      time.Now(),
		FromVersion:    fromVersion,
		ToVersion:      toVersion,
		State:          StateDownloading,
		DeploymentMode: deploymentMode,
	}
	m.current = entry
	return entry, nil
}

// Transition moves the update to a new state.
func (m *Manager) Transition(id string, newState State) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || m.current.ID != id {
		return errors.New("no matching update in progress")
	}

	valid := map[State][]State{
		StateIdle:        {StateDownloading},
		StateDownloading: {StateVerifying, StateFailed},
		StateVerifying:   {StateInstalling, StateFailed},
		StateInstalling:  {StateReadiness, StateFailed},
		StateReadiness:   {StateCompleted, StateRolledBack, StateFailed},
	}

	allowed := valid[m.current.State]
	validTransition := false
	for _, s := range allowed {
		if s == newState {
			validTransition = true
			break
		}
	}
	if !validTransition {
		return fmt.Errorf("invalid transition from %s to %s", m.current.State, newState)
	}

	m.current.State = newState
	return nil
}

// Fail marks the update as failed with an error message and triggers rollback.
func (m *Manager) Fail(id string, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || m.current.ID != id {
		return errors.New("no matching update in progress")
	}

	m.current.State = StateFailed
	m.current.Error = err.Error()
	m.current.CompletedAt = time.Now()
	m.archive()
	return nil
}

// Complete marks the update as successfully completed.
func (m *Manager) Complete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || m.current.ID != id {
		return errors.New("no matching update in progress")
	}

	m.current.State = StateCompleted
	m.current.CompletedAt = time.Now()
	m.current.ReadinessOK = true
	m.archive()
	return nil
}

// Rollback marks the update as rolled back.
func (m *Manager) Rollback(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || m.current.ID != id {
		return errors.New("no matching update in progress")
	}

	m.current.State = StateRolledBack
	m.current.CompletedAt = time.Now()
	m.current.RollbackUsed = true
	m.archive()
	return nil
}

// archive moves the current entry to history and enforces the max history limit.
func (m *Manager) archive() {
	if m.current != nil {
		m.history = append(m.history, *m.current)
		if len(m.history) > m.config.MaxHistory {
			m.history = m.history[len(m.history)-m.config.MaxHistory:]
		}
	}
	m.current = nil
}

// Current returns the current update entry, or nil if no update is in progress.
func (m *Manager) Current() *Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return nil
	}
	entry := *m.current
	return &entry
}

// History returns a copy of the update history.
func (m *Manager) History() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Entry, len(m.history))
	copy(result, m.history)
	return result
}

// IsUpdating returns true if an update is currently in progress.
func (m *Manager) IsUpdating() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != nil && m.current.State != StateCompleted && m.current.State != StateRolledBack && m.current.State != StateFailed
}

// ReadinessConfig returns the readiness check configuration.
func (m *Manager) ReadinessConfig() (timeout, interval time.Duration) {
	return m.config.ReadinessTimeout, m.config.ReadinessInterval
}
