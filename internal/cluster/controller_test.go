package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/node"
)

type memoryStore struct {
	state  State
	has    bool
	tokens map[string]time.Time
}

func (s *memoryStore) SaveClusterState(_ context.Context, v State) error {
	s.state = v
	s.has = true
	return nil
}
func (s *memoryStore) GetClusterState(context.Context) (State, error) {
	if !s.has {
		return State{}, errors.New("missing")
	}
	return s.state, nil
}
func (s *memoryStore) HasClusterState(context.Context) (bool, error) { return s.has, nil }
func (s *memoryStore) CreateClusterJoinToken(_ context.Context, d []byte, e time.Time) error {
	if s.tokens == nil {
		s.tokens = map[string]time.Time{}
	}
	s.tokens[string(d)] = e
	return nil
}
func (s *memoryStore) ConsumeClusterJoinToken(context.Context, []byte) error { return nil }
func (s *memoryStore) SaveNode(context.Context, node.Node) error             { return nil }
func TestControllerCreatesClusterAndJoinBundle(t *testing.T) {
	s := &memoryStore{}
	c := NewController(s)
	c.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	state, err := c.Create(context.Background(), "leader", "127.0.0.1:9443")
	if err != nil || state.Role != "leader" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	bundle, err := c.CreateJoinBundle(context.Background())
	if err != nil || bundle.Token == "" || len(bundle.CACertificate) == 0 {
		t.Fatalf("bundle=%+v err=%v", bundle, err)
	}
}
