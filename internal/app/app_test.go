package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/config"
)

func TestRunRejectsInvalidConfigurationBeforeStartingResources(t *testing.T) {
	err := Run(context.Background(), config.Config{}, "", slog.New(slog.NewTextHandler(io.Discard, nil)), api.Version{})
	if err == nil {
		t.Fatal("invalid configuration was accepted")
	}
}

func TestCacheUpstreamTTLBounds(t *testing.T) {
	for _, seconds := range []int{-1, 604801} {
		if _, err := cacheUpstreamTTL(seconds); err == nil {
			t.Fatalf("accepted invalid cache upstream TTL %d", seconds)
		}
	}
	if got, err := cacheUpstreamTTL(604800); err != nil || got != 604800 {
		t.Fatalf("cache upstream TTL = %d, %v", got, err)
	}
}
