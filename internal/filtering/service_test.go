package filtering

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	domains []string
	err     error
}

func (m *memoryStore) LoadBlocklistDomains(context.Context) ([]string, error) {
	return m.domains, m.err
}
func TestServiceKeepsSnapshotWhenReloadFails(t *testing.T) {
	store := &memoryStore{domains: []string{"ads.example"}}
	service, err := NewService(context.Background(), store, nil)
	if err != nil || !service.Blocked("a.ads.example") {
		t.Fatal(err)
	}
	store.domains = []string{"other.example"}
	store.err = errors.New("failed")
	if service.Reload(context.Background()) == nil || !service.Blocked("a.ads.example") || service.Blocked("other.example") {
		t.Fatal("failed reload changed snapshot")
	}
}
