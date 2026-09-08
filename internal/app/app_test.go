package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/config"
)

func TestNewDatabaseRequiresExplicitBootstrapPassword(t *testing.T) {
	c := config.Default()
	c.DatabasePath = filepath.Join(t.TempDir(), "new.db")
	err := Run(context.Background(), c, slog.New(slog.NewTextHandler(io.Discard, nil)), api.Version{})
	if err == nil || !strings.Contains(err.Error(), "VELORA_BOOTSTRAP_PASSWORD") {
		t.Fatalf("startup did not fail securely: %v", err)
	}
}
