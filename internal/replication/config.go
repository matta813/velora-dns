// Package replication implements configuration replication between nodes.
package replication

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ConfigVersion represents a versioned configuration.
type ConfigVersion struct {
	Version    int       `json:"version"`
	ConfigHash string    `json:"config_hash"`
	Config     any       `json:"config"`
	AppliedBy  string    `json:"applied_by"`
	AppliedAt  time.Time `json:"applied_at"`
}

// ConfigStore manages config version persistence.
type ConfigStore interface {
	SaveConfigVersion(ctx context.Context, v ConfigVersion) error
	GetLatestVersion(ctx context.Context) (ConfigVersion, error)
	ListVersions(ctx context.Context, limit int) ([]ConfigVersion, error)
}

// ConfigReplicator handles versioned configuration replication.
type ConfigReplicator struct {
	store      ConfigStore
	currentVer int
	mu         sync.RWMutex
	logger     Logger
}

// Logger is a simple logger interface.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
}

// NewConfigReplicator creates a new config replicator.
func NewConfigReplicator(store ConfigStore, logger Logger) *ConfigReplicator {
	return &ConfigReplicator{
		store:  store,
		logger: logger,
	}
}

// Start initializes the replicator.
func (r *ConfigReplicator) Start(ctx context.Context) {
	latest, err := r.store.GetLatestVersion(ctx)
	if err == nil {
		r.currentVer = latest.Version
	}
}

// Replicate sends a configuration update to all connected nodes.
func (r *ConfigReplicator) Replicate(ctx context.Context, config any, appliedBy string) (*ConfigVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	hash := sha256.Sum256(configJSON)
	r.currentVer++

	v := ConfigVersion{
		Version:    r.currentVer,
		ConfigHash: fmt.Sprintf("%x", hash),
		Config:     config,
		AppliedBy:  appliedBy,
		AppliedAt:  time.Now(),
	}

	if err := r.store.SaveConfigVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("save config version: %w", err)
	}

	r.logger.Info("config replicated", "version", v.Version, "hash", v.ConfigHash)
	return &v, nil
}

// GetVersion returns a specific config version.
func (r *ConfigReplicator) GetVersion(ctx context.Context, version int) (*ConfigVersion, error) {
	versions, err := r.store.ListVersions(ctx, 100)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.Version == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version not found: %d", version)
}

// GetCurrentVersion returns the current config version.
func (r *ConfigReplicator) GetCurrentVersion() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentVer
}

// GetHistory returns recent config versions.
func (r *ConfigReplicator) GetHistory(ctx context.Context, limit int) ([]ConfigVersion, error) {
	return r.store.ListVersions(ctx, limit)
}
