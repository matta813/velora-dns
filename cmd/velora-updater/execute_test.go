package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyChecksumAndCopyHelpers(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	contents := []byte("release payload")
	if err := os.WriteFile(source, contents, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	checksum := filepath.Join(dir, "source.sha256")
	if err := os.WriteFile(checksum, []byte(fmt.Sprintf("%x  source\n", digest)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(source, checksum); err != nil {
		t.Fatalf("verifyChecksum() = %v", err)
	}
	if err := os.WriteFile(checksum, []byte("000000000000000000000000000000000000000000000000000000000000"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(source, checksum); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}

	destination := filepath.Join(dir, "destination")
	if err := copyFile(source, destination, 0755); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != string(contents) {
		t.Fatalf("copied file = %q, %v", got, err)
	}
}

func TestBackupRestoreAndInstallRelease(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "velora-dns")
	web := filepath.Join(dir, "web")
	if err := os.WriteFile(binary, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "old.txt"), []byte("old web"), 0644); err != nil {
		t.Fatal(err)
	}
	agent := &Agent{config: AgentConfig{BinaryPath: binary, WebDir: web}}
	if err := agent.backupCurrent(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary + defaultBackupSuffix); err != nil {
		t.Fatal(err)
	}
	if err := agent.restoreBackup(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(binary); string(got) != "old binary" {
		t.Fatalf("restored binary = %q", got)
	}

	bundle := filepath.Join(dir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "web-dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "velora-dns"), []byte("new binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "web-dist", "index.html"), []byte("new web"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VELORA_UPDATE_BUNDLE_PATH", bundle)
	if err := agent.installRelease(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(binary); string(got) != "new binary" {
		t.Fatalf("installed binary = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(web, "index.html")); string(got) != "new web" {
		t.Fatalf("installed web asset = %q", got)
	}
}
