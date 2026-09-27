package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/matta813/velora-dns/internal/rewrites"
)

func TestRewritesPersist(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "velora.db")
	store, err := Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.SaveRewrite(ctx, rewrites.Rule{Name: "*.lab.home", Type: "A", Value: "192.0.2.10", Enabled: true, Description: "Lab"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveRewrite(ctx, rewrites.Rule{Name: "*.lab.home", Type: "A", Value: "192.0.2.10"}); !errors.Is(err, rewrites.ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err = store.SaveRewrite(ctx, rewrites.Rule{Name: "*.lab.home", Type: "AAAA", Value: "2001:db8::10", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	a.Enabled = false
	if _, err = store.SaveRewrite(ctx, a); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if store, err = Open(ctx, "sqlite", path); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	rules, err := store.LoadRewrites(ctx)
	if err != nil || len(rules) != 2 || rules[0].Type != "A" || rules[0].Enabled || rules[0].Description != "Lab" || rules[1].Value != "2001:db8::10" {
		t.Fatalf("round trip: %v %+v", err, rules)
	}
	if err = store.DeleteRewrite(ctx, 999); !errors.Is(err, rewrites.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}
