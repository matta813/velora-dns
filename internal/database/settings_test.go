package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRateLimitSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	got, err := store.GetRateLimitSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.GlobalQPS != 0 || got.ClientQPS != 0 || got.RateLimitBurst != 0 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	want := RateLimitSettings{Enabled: true, GlobalQPS: 500, ClientQPS: 50, RateLimitBurst: 25}
	if err = store.SetRateLimitSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetRateLimitSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.GlobalQPS != 500 || got.ClientQPS != 50 || got.RateLimitBurst != 25 {
		t.Fatalf("round trip: %+v", got)
	}
	if err = store.SetRateLimitSettings(ctx, RateLimitSettings{Enabled: true, GlobalQPS: 0, ClientQPS: 50, RateLimitBurst: 25}); err == nil {
		t.Fatal("invalid rate limit settings accepted")
	}
}
