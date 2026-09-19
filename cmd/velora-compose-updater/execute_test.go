package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/update"
)

func TestComposeCommandsAndMissingReleaseImage(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "compose.log")
	command := filepath.Join(dir, "compose")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+log+"'\ncase \" $* \" in *' ps '*) echo sha256:current;; esac\n"), 0755); err != nil {
		t.Fatal(err)
	}
	agent := &Agent{config: AgentConfig{ComposePath: command, ComposeDir: dir, ComposeProject: "velora", ServiceName: "dns"}, manager: update.NewManager(update.DefaultConfig())}
	ctx := context.Background()
	if got := agent.getCurrentDigest(ctx); got != "sha256:current" {
		t.Fatalf("digest = %q", got)
	}
	if err := agent.pullImage(ctx); err == nil {
		t.Fatal("missing release image was accepted")
	}
	t.Setenv("VELORA_RELEASE_IMAGE", "ghcr.io/example/velora@sha256:abc")
	if err := agent.pullImage(ctx); err != nil {
		t.Fatal(err)
	}
	if err := agent.updateService(ctx); err != nil {
		t.Fatal(err)
	}
	if err := agent.restorePreviousDigest(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := agent.restorePreviousDigest(ctx, "sha256:current"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-p velora ps -q dns", "-p velora pull dns", "-p velora up -d --no-deps dns"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("command log missing %q:\n%s", want, got)
		}
	}
}
