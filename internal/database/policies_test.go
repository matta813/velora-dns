package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/policies"
)

func TestPoliciesPersistAndFollowClients(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "velora.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	client, err := store.SaveClient(ctx, clients.Client{Name: "Kid tablet", Addresses: []string{"192.168.1.40"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.SavePolicy(ctx, policies.Policy{ClientID: client.ID, Mode: policies.ModeCustom, Blocklists: []int64{3, 7}, Allow: []string{"school.example"}, Block: []string{"games.example", "video.example"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SavePolicy(ctx, policies.Policy{ClientID: client.ID, Mode: policies.ModeDefault}); !errors.Is(err, policies.ErrExists) {
		t.Fatalf("one policy per client: %v", err)
	}
	if _, err = store.SavePolicy(ctx, policies.Policy{ClientID: 999, Mode: policies.ModeDefault}); err == nil {
		t.Fatal("unknown clients must be rejected")
	}
	loaded, err := store.LoadPolicies(ctx)
	if err != nil || len(loaded) != 1 || loaded[0].ID != saved.ID || len(loaded[0].Blocklists) != 2 || loaded[0].Blocklists[1] != 7 || len(loaded[0].Block) != 2 || loaded[0].Allow[0] != "school.example" {
		t.Fatalf("load: %v %+v", err, loaded)
	}
	saved.Mode, saved.Blocklists, saved.Allow, saved.Block = policies.ModeDisabled, nil, nil, nil
	if _, err = store.SavePolicy(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteClient(ctx, client.ID); err != nil {
		t.Fatal(err)
	}
	if loaded, err = store.LoadPolicies(ctx); err != nil || len(loaded) != 0 {
		t.Fatalf("policy should follow its client: %v %+v", err, loaded)
	}
	if err = store.DeletePolicy(ctx, saved.ID); !errors.Is(err, policies.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}
