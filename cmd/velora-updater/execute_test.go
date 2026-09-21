package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if err := os.WriteFile(checksum, []byte("0000000000000000000000000000000000000000000000000000000000000000  source\n"), 0600); err != nil {
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

func TestDownloadAndArchiveSafety(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/failed" {
			http.Error(w, "no", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "download")
	if err := downloadFile(context.Background(), server.URL+"/ok", destination, 64); err != nil {
		t.Fatal(err)
	}
	if err := downloadFile(context.Background(), server.URL+"/failed", destination+"-failed", 64); err == nil {
		t.Fatal("download failure was accepted")
	}
	if err := downloadFile(context.Background(), server.URL+"/ok", destination+"-small", 2); err == nil {
		t.Fatal("oversized download was accepted")
	}

	archive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0600, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()
	if _, err := extractBundle(archive); err == nil {
		t.Fatal("unsafe archive path was accepted")
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
	if err := os.WriteFile(filepath.Join(bundle, "VERSION"), []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "web-dist", "index.html"), []byte("new web"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := agent.installRelease(bundle); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(binary); string(got) != "new binary" {
		t.Fatalf("installed binary = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(web, "index.html")); string(got) != "new web" {
		t.Fatalf("installed web asset = %q", got)
	}
}
