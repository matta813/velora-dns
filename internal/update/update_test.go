package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerBeginTransitionComplete(t *testing.T) {
	m := NewManager(Config{
		ReadinessTimeout:  10 * time.Second,
		ReadinessInterval: 100 * time.Millisecond,
		MaxHistory:        5,
	})

	entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if entry.FromVersion != "0.1.0" || entry.ToVersion != "0.2.0" {
		t.Fatalf("unexpected versions: %s -> %s", entry.FromVersion, entry.ToVersion)
	}
	if entry.State != StateDownloading {
		t.Fatalf("expected state downloading, got %s", entry.State)
	}
	if !m.IsUpdating() {
		t.Fatal("expected IsUpdating to return true")
	}

	if err := m.Transition(entry.ID, StateVerifying); err != nil {
		t.Fatalf("Transition to verifying: %v", err)
	}
	if err := m.Transition(entry.ID, StateInstalling); err != nil {
		t.Fatalf("Transition to installing: %v", err)
	}
	if err := m.Transition(entry.ID, StateReadiness); err != nil {
		t.Fatalf("Transition to readiness: %v", err)
	}
	if err := m.Complete(entry.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if m.IsUpdating() {
		t.Fatal("expected IsUpdating to return false after completion")
	}
	if m.Current() != nil {
		t.Fatal("expected no current entry after completion")
	}
	if len(m.History()) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(m.History()))
	}
	if m.History()[0].State != StateCompleted {
		t.Fatalf("expected history state completed, got %s", m.History()[0].State)
	}
}

func TestManagerConcurrentUpdateRejected(t *testing.T) {
	m := NewManager(Config{})

	entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	_, err = m.Begin("0.2.0", "0.3.0", "systemd")
	if err == nil {
		t.Fatal("expected concurrent update to be rejected")
	}

	_ = m.Complete(entry.ID)
}

func TestManagerInvalidTransition(t *testing.T) {
	m := NewManager(Config{})

	entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	err = m.Transition(entry.ID, StateCompleted)
	if err == nil {
		t.Fatal("expected invalid transition to be rejected")
	}
}

func TestManagerFailAndRollback(t *testing.T) {
	m := NewManager(Config{MaxHistory: 3})

	entry, err := m.Begin("0.1.0", "0.2.0", "compose")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := m.Transition(entry.ID, StateVerifying); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if err := m.Fail(entry.ID, checksumError{version: "0.2.0"}); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if len(m.History()) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(m.History()))
	}
	if m.History()[0].State != StateFailed {
		t.Fatalf("expected history state failed, got %s", m.History()[0].State)
	}

	entry2, err := m.Begin("0.1.0", "0.3.0", "compose")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := m.Rollback(entry2.ID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(m.History()) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(m.History()))
	}
	if !m.History()[1].RollbackUsed {
		t.Fatal("expected rollback_used to be true")
	}
}

func TestManagerMaxHistory(t *testing.T) {
	m := NewManager(Config{MaxHistory: 3})

	for i := 0; i < 5; i++ {
		entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := m.Complete(entry.ID); err != nil {
			t.Fatalf("Complete %d: %v", i, err)
		}
	}

	if len(m.History()) != 3 {
		t.Fatalf("expected 3 history entries (max), got %d", len(m.History()))
	}
}

func TestManagerPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	m1 := NewManager(Config{
		MaxHistory: 5,
		StateFile:  stateFile,
	})

	entry, err := m1.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := m1.Transition(entry.ID, StateVerifying); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if err := m1.SaveState(); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	m2 := NewManager(Config{
		MaxHistory: 5,
		StateFile:  stateFile,
	})
	if err := m2.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	current := m2.Current()
	if current == nil {
		t.Fatal("expected current entry after load")
	}
	if current.ID != entry.ID {
		t.Fatalf("expected ID %s, got %s", entry.ID, current.ID)
	}
	if current.State != StateVerifying {
		t.Fatalf("expected state verifying, got %s", current.State)
	}
}

func TestManagerLoadNonexistentState(t *testing.T) {
	m := NewManager(Config{StateFile: "/nonexistent/path/state.json"})
	if err := m.LoadState(); err != nil {
		t.Fatalf("LoadState should not fail for nonexistent file: %v", err)
	}
}

func TestManagerReadinessConfig(t *testing.T) {
	m := NewManager(Config{
		ReadinessTimeout:  30 * time.Second,
		ReadinessInterval: 2 * time.Second,
	})

	timeout, interval := m.ReadinessConfig()
	if timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %v", timeout)
	}
	if interval != 2*time.Second {
		t.Fatalf("expected interval 2s, got %v", interval)
	}
}

func TestManagerNoMatchingID(t *testing.T) {
	m := NewManager(Config{})

	_, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	err = m.Transition("nonexistent-id", StateVerifying)
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestManagerMultipleHistories(t *testing.T) {
	m := NewManager(Config{MaxHistory: 10})

	for i := 0; i < 5; i++ {
		entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := m.Complete(entry.ID); err != nil {
			t.Fatalf("Complete %d: %v", i, err)
		}
	}

	history := m.History()
	if len(history) != 5 {
		t.Fatalf("expected 5 history entries, got %d", len(history))
	}

	for i, h := range history {
		if h.State != StateCompleted {
			t.Fatalf("history[%d]: expected state completed, got %s", i, h.State)
		}
	}
}

type checksumError struct{ version string }

func (e checksumError) Error() string {
	return "checksum mismatch for " + e.version
}

func TestStateConstants(t *testing.T) {
	states := []State{
		StateIdle, StateDownloading, StateVerifying, StateInstalling,
		StateReadiness, StateCompleted, StateRolledBack, StateFailed,
	}
	seen := make(map[State]bool)
	for _, s := range states {
		if seen[s] {
			t.Fatalf("duplicate state constant: %s", s)
		}
		seen[s] = true
		if s == "" {
			t.Fatal("empty state constant")
		}
	}
}

func TestManagerHistoryCopy(t *testing.T) {
	m := NewManager(Config{MaxHistory: 10})

	entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	_ = m.Complete(entry.ID)

	h1 := m.History()
	h1[0].State = StateFailed

	h2 := m.History()
	if h2[0].State != StateCompleted {
		t.Fatal("modifying returned history should not affect internal state")
	}
}

func TestStateFilePermissions(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	m := NewManager(Config{
		StateFile: stateFile,
	})

	entry, err := m.Begin("0.1.0", "0.2.0", "systemd")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	_ = m.Complete(entry.ID)

	if err := m.SaveState(); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	info, err := os.Stat(stateFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0640 {
		t.Fatalf("expected permissions 0640, got %04o", perm)
	}
}
