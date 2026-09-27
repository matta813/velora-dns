package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/matta813/velora-dns/internal/webhooks"
)

func TestWebhooksPersist(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "velora.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	saved, err := store.SaveWebhook(ctx, webhooks.Hook{Name: "Home Assistant", URL: "http://192.168.1.5:8123/api/webhook/velora", Events: []string{"upstream.unavailable", "backup.failed"}, MinSeverity: "warning", AllowPrivate: true, Token: "t0ken", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveWebhook(ctx, webhooks.Hook{Name: "home assistant", URL: "https://a.example/", MinSeverity: "info"}); !errors.Is(err, webhooks.ErrExists) {
		t.Fatalf("unique name: %v", err)
	}
	loaded, err := store.LoadWebhooks(ctx)
	if err != nil || len(loaded) != 1 || loaded[0].ID != saved.ID || len(loaded[0].Events) != 2 || !loaded[0].AllowPrivate || loaded[0].Token != "t0ken" || loaded[0].MinSeverity != "warning" {
		t.Fatalf("load: %v %+v", err, loaded)
	}
	saved.Enabled, saved.Token = false, ""
	if _, err = store.SaveWebhook(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteWebhook(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteWebhook(ctx, saved.ID); !errors.Is(err, webhooks.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}
