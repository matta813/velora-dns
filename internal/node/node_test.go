package node

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type testStore struct {
	mu    sync.Mutex
	saved []Node
	err   error
}

func (s *testStore) SaveNode(_ context.Context, n Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, n)
	return s.err
}
func (s *testStore) GetNode(context.Context, string) (Node, error) {
	return Node{}, errors.New("not found")
}
func (s *testStore) ListNodes(context.Context) ([]Node, error) { return nil, nil }
func (s *testStore) DeleteNode(context.Context, string) error  { return nil }

type testLogger struct{}

func (testLogger) Info(string, ...any)  {}
func (testLogger) Error(string, ...any) {}
func (testLogger) Warn(string, ...any)  {}

func TestMembershipRegistersAndManagesPeers(t *testing.T) {
	store := &testStore{}
	m := NewMembership(store, Node{ID: "local"}, testLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if len(store.saved) != 1 || store.saved[0].ID != "local" {
		t.Fatalf("local node was not registered: %+v", store.saved)
	}
	m.AddPeer(ctx, Node{ID: "peer", Name: "secondary"})
	m.AddPeer(ctx, Node{ID: "peer", Name: "duplicate"})
	if peers := m.GetPeers(); len(peers) != 1 || peers[0].Name != "secondary" || !m.IsHealthy() {
		t.Fatalf("unexpected peers: %+v, healthy=%t", peers, m.IsHealthy())
	}
	m.RemovePeer("peer")
	if len(m.GetPeers()) != 0 {
		t.Fatal("peer was not removed")
	}
}

func TestGenerateNodeID(t *testing.T) {
	id, err := GenerateNodeID()
	if err != nil || len(id) != 32 {
		t.Fatalf("GenerateNodeID() = %q, %v", id, err)
	}
}
