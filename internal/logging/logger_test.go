package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewHonorsConfiguredLevel(t *testing.T) {
	var out bytes.Buffer
	logger := New(&out, "warn")
	logger.Info("hidden")
	logger.Warn("shown")

	got := out.String()
	if strings.Contains(got, "hidden") || !strings.Contains(got, "shown") || !strings.Contains(got, `"level":"WARN"`) {
		t.Fatalf("unexpected log output: %s", got)
	}
}

func TestNewUsesInfoForUnknownLevel(t *testing.T) {
	var out bytes.Buffer
	New(&out, "invalid").Info("shown")
	if !strings.Contains(out.String(), "shown") {
		t.Fatalf("info message missing from output: %s", out.String())
	}
}
