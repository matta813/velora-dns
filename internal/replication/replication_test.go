package replication

import (
	"context"
	"errors"
	"testing"
)

type testConfigStore struct {
	versions []ConfigVersion
	err      error
}

func (s *testConfigStore) SaveConfigVersion(_ context.Context, v ConfigVersion) error {
	if s.err != nil {
		return s.err
	}
	s.versions = append(s.versions, v)
	return nil
}
func (s *testConfigStore) GetLatestVersion(context.Context) (ConfigVersion, error) {
	if len(s.versions) == 0 {
		return ConfigVersion{}, errors.New("empty")
	}
	return s.versions[len(s.versions)-1], nil
}
func (s *testConfigStore) ListVersions(context.Context, int) ([]ConfigVersion, error) {
	return s.versions, s.err
}

type testLogger struct{}

func (testLogger) Info(string, ...any)  {}
func (testLogger) Error(string, ...any) {}
func (testLogger) Warn(string, ...any)  {}

func TestConfigReplicatorVersionsAndFindsHistory(t *testing.T) {
	store := &testConfigStore{versions: []ConfigVersion{{Version: 3}}}
	replicator := NewConfigReplicator(store, testLogger{})
	replicator.Start(context.Background())
	version, err := replicator.Replicate(context.Background(), map[string]string{"dns": "changed"}, "operator")
	if err != nil || version.Version != 4 || version.ConfigHash == "" || replicator.GetCurrentVersion() != 4 {
		t.Fatalf("unexpected version: %+v, err=%v", version, err)
	}
	found, err := replicator.GetVersion(context.Background(), 4)
	if err != nil || found.AppliedBy != "operator" {
		t.Fatalf("GetVersion() = %+v, %v", found, err)
	}
	if _, err := replicator.GetVersion(context.Background(), 99); err == nil {
		t.Fatal("missing version did not return an error")
	}
}

type testMembership struct{ peers []ManagedNodeInfo }

func (m testMembership) GetPeers() []ManagedNodeInfo { return m.peers }
func (testMembership) IsHealthy() bool               { return true }

type testReplicator struct {
	called bool
	err    error
}

func (r *testReplicator) Replicate(context.Context, any, string) (*ConfigVersion, error) {
	r.called = true
	return &ConfigVersion{Version: 1}, r.err
}
func (testReplicator) GetCurrentVersion() int { return 1 }

func TestClusterManagerUpdatesNodesAndRollsOut(t *testing.T) {
	replicator := &testReplicator{}
	manager := NewClusterManager(testMembership{peers: []ManagedNodeInfo{{ID: "node-1", Name: "one", Address: "10.0.0.1", Status: "healthy"}}}, replicator, testLogger{})
	manager.updateNodes(context.Background())
	node, ok := manager.GetNode("node-1")
	if !ok || node.Status != "healthy" || node.LastHeartbeat.IsZero() {
		t.Fatalf("unexpected node: %+v, exists=%t", node, ok)
	}
	if err := manager.RolloutConfig(context.Background(), map[string]bool{"enabled": true}); err != nil || !replicator.called {
		t.Fatalf("RolloutConfig() err=%v called=%t", err, replicator.called)
	}
	if err := NewClusterManager(testMembership{}, replicator, testLogger{}).RolloutConfig(context.Background(), nil); err == nil {
		t.Fatal("empty rollout did not return an error")
	}
}
