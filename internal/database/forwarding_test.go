package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/matta813/velora-dns/internal/forwarding"
)

func TestForwardRulesPersist(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "velora.db")
	store, err := Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := store.SaveForwardRule(ctx, forwarding.Rule{Domain: "corp.example", Upstreams: []string{"10.0.0.10:53", "[2001:db8::53]:53"}, Enabled: true, Description: "Office AD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveForwardRule(ctx, forwarding.Rule{Domain: "corp.example", Upstreams: []string{"10.0.0.11:53"}}); !errors.Is(err, forwarding.ErrExists) {
		t.Fatalf("duplicate domain: %v", err)
	}
	rule.Enabled = false
	if _, err = store.SaveForwardRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveForwardRule(ctx, forwarding.Rule{ID: 999, Domain: "x.example", Upstreams: []string{"10.0.0.1:53"}}); !errors.Is(err, forwarding.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	_ = store.Close()
	if store, err = Open(ctx, "sqlite", path); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	rules, err := store.LoadForwardRules(ctx)
	if err != nil || len(rules) != 1 {
		t.Fatalf("load: %v %+v", err, rules)
	}
	got := rules[0]
	if got.Enabled || got.Description != "Office AD" || len(got.Upstreams) != 2 || got.Upstreams[1] != "[2001:db8::53]:53" {
		t.Fatalf("round trip: %+v", got)
	}
	if err = store.DeleteForwardRule(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteForwardRule(ctx, got.ID); !errors.Is(err, forwarding.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}
