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
	err := Run(context.Background(), config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), api.Version{})
	if err == nil {
		t.Fatal("invalid configuration was accepted")
	}
}
