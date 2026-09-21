package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

func (a *Agent) executeUpdate(entry *update.Entry, release update.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.Timeout)
	defer cancel()
	log := a.config.Logger.With("update_id", entry.ID)

	bundlePath, cleanup, err := a.downloadRelease(ctx, release)
	if err != nil {
		_ = a.manager.Fail(entry.ID, fmt.Errorf("download: %w", err))
		_ = a.saveState()
		return
	}
	defer cleanup()

	if err := a.manager.Transition(entry.ID, update.StateVerifying); err != nil {
		log.Error("transition to verifying failed", "error", err)
		return
	}
	if err := verifyChecksum(bundlePath, bundlePath+".sha256sum"); err != nil {
		_ = a.manager.Fail(entry.ID, fmt.Errorf("checksum verification: %w", err))
		_ = a.saveState()
		return
	}
	extracted, err := extractBundle(bundlePath)
	if err != nil {
		_ = a.manager.Fail(entry.ID, fmt.Errorf("extract release: %w", err))
		_ = a.saveState()
		return
	}
	if err := a.saveState(); err != nil {
		log.Error("save state failed", "error", err)
	}

	if err := a.manager.Transition(entry.ID, update.StateInstalling); err != nil {
		log.Error("transition to installing failed", "error", err)
		return
	}

	if err := a.backupCurrent(); err != nil {
		log.Error("backup failed", "error", err)
		_ = a.manager.Fail(entry.ID, fmt.Errorf("backup: %w", err))
		_ = a.saveState()
		return
	}

	if err := a.installRelease(extracted); err != nil {
		log.Error("install failed", "error", err)
		if rbErr := a.restoreBackup(); rbErr != nil {
			log.Error("restore also failed", "error", rbErr)
			_ = a.manager.Fail(entry.ID, fmt.Errorf("install failed: %v, restore failed: %v", err, rbErr))
		} else {
			_ = a.manager.RollbackWithError(entry.ID, fmt.Errorf("install failed: %w", err))
		}
		_ = a.saveState()
		return
	}
	if err := a.restartService(); err != nil {
		log.Error("restart failed", "error", err)
		if rbErr := a.restoreAndRestart(); rbErr != nil {
			_ = a.manager.Fail(entry.ID, fmt.Errorf("restart failed: %v, rollback failed: %v", err, rbErr))
		} else {
			_ = a.manager.RollbackWithError(entry.ID, fmt.Errorf("restart failed: %w", err))
		}
		_ = a.saveState()
		return
	}

	if err := a.manager.Transition(entry.ID, update.StateReadiness); err != nil {
		log.Error("transition to readiness failed", "error", err)
		return
	}

	ready := a.waitForReadiness(ctx)
	if !ready {
		log.Error("readiness check failed, rolling back")
		if rbErr := a.restoreAndRestart(); rbErr != nil {
			log.Error("restore also failed", "error", rbErr)
			_ = a.manager.Fail(entry.ID, fmt.Errorf("readiness failed, restore failed: %v", rbErr))
		} else if !a.waitForReadiness(context.Background()) {
			_ = a.manager.Fail(entry.ID, fmt.Errorf("readiness failed and restored version did not recover"))
		} else {
			_ = a.manager.RollbackWithError(entry.ID, fmt.Errorf("new version failed readiness"))
		}
		_ = a.saveState()
		return
	}

	if err := a.manager.Complete(entry.ID); err != nil {
		log.Error("complete failed", "error", err)
		return
	}
	_ = a.saveState()
	log.Info("update completed successfully")
}

func (a *Agent) backupCurrent() error {
	binPrev := a.config.BinaryPath + defaultBackupSuffix
	webPrev := a.config.WebDir + defaultBackupSuffix
	versionPath := filepath.Join(filepath.Dir(a.config.BinaryPath), "VERSION")
	_ = os.Remove(versionPath + defaultBackupSuffix)
	if err := os.Rename(versionPath, versionPath+defaultBackupSuffix); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup version: %w", err)
	}

	_ = os.Remove(binPrev)
	if err := os.Rename(a.config.BinaryPath, binPrev); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup binary: %w", err)
	}

	_ = os.RemoveAll(webPrev)
	if info, err := os.Stat(a.config.WebDir); err == nil && info.IsDir() {
		if err := os.Rename(a.config.WebDir, webPrev); err != nil {
			return fmt.Errorf("backup web dir: %w", err)
		}
	}
	return nil
}

func (a *Agent) restoreBackup() error {
	binPrev := a.config.BinaryPath + defaultBackupSuffix
	webPrev := a.config.WebDir + defaultBackupSuffix
	versionPath := filepath.Join(filepath.Dir(a.config.BinaryPath), "VERSION")

	if _, err := os.Stat(binPrev); err == nil {
		_ = os.Remove(a.config.BinaryPath)
		if err := os.Rename(binPrev, a.config.BinaryPath); err != nil {
			return fmt.Errorf("restore binary: %w", err)
		}
	}

	if _, err := os.Stat(webPrev); err == nil {
		_ = os.RemoveAll(a.config.WebDir)
		if err := os.Rename(webPrev, a.config.WebDir); err != nil {
			return fmt.Errorf("restore web dir: %w", err)
		}
	}
	if _, err := os.Stat(versionPath + defaultBackupSuffix); err == nil {
		_ = os.Remove(versionPath)
		if err := os.Rename(versionPath+defaultBackupSuffix, versionPath); err != nil {
			return fmt.Errorf("restore version: %w", err)
		}
	}
	return nil
}

func (a *Agent) installRelease(bundlePath string) error {
	binSrc := filepath.Join(bundlePath, "velora-dns")
	if _, err := os.Stat(binSrc); err == nil {
		if err := copyFile(binSrc, a.config.BinaryPath, 0755); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
	}
	versionSrc := filepath.Join(bundlePath, "VERSION")
	if _, err := os.Stat(versionSrc); err != nil {
		return fmt.Errorf("release VERSION: %w", err)
	}
	if err := copyFile(versionSrc, filepath.Join(filepath.Dir(a.config.BinaryPath), "VERSION"), 0644); err != nil {
		return fmt.Errorf("install VERSION: %w", err)
	}

	webSrc := filepath.Join(bundlePath, "web-dist")
	if info, err := os.Stat(webSrc); err == nil && info.IsDir() {
		if err := copyDir(webSrc, a.config.WebDir); err != nil {
			return fmt.Errorf("install web assets: %w", err)
		}
	}

	serviceSrc := filepath.Join(bundlePath, "velora-dns.service")
	if _, err := os.Stat(serviceSrc); err == nil {
		if err := copyFile(serviceSrc, "/etc/systemd/system/velora-dns.service", 0644); err != nil {
			return fmt.Errorf("install service: %w", err)
		}
	}

	return nil
}

func (a *Agent) downloadRelease(ctx context.Context, release update.Release) (string, func(), error) {
	dir, err := os.MkdirTemp("", "velora-update-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	artifactName := fmt.Sprintf("velora-dns-%s-%s.tar.gz", release.Version, strings.ReplaceAll(release.Architecture, "/", "-"))
	artifact := filepath.Join(dir, artifactName)
	if err := downloadFile(ctx, release.ArtifactURL, artifact, 512<<20); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := downloadFile(ctx, release.ChecksumURL, artifact+".sha256sum", 1<<20); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return artifact, cleanup, nil
}

func downloadFile(ctx context.Context, source, destination string, limit int64) error {
	if source == "" {
		return errors.New("release asset URL is missing")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("download exceeds size limit")
	}
	return out.Close()
}

func extractBundle(archive string) (string, error) {
	destination := strings.TrimSuffix(archive, ".tar.gz")
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", err
	}
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var root string
	var extractedBytes int64
	const maxExtractedBytes = 1 << 30
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		clean := filepath.Clean(header.Name)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", errors.New("unsafe path in release archive")
		}
		target := filepath.Join(destination, clean)
		if !strings.HasPrefix(target, destination+string(os.PathSeparator)) {
			return "", errors.New("unsafe path in release archive")
		}
		if root == "" {
			root = strings.Split(clean, string(os.PathSeparator))[0]
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > maxExtractedBytes-extractedBytes {
				return "", errors.New("release archive exceeds extracted size limit")
			}
			extractedBytes += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(header.Mode)&0777)
			if err != nil {
				return "", err
			}
			written, copyErr := io.CopyN(out, reader, header.Size)
			closeErr := out.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if written != header.Size {
				return "", errors.New("truncated release archive entry")
			}
			if closeErr != nil {
				return "", closeErr
			}
		default:
			return "", fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
	if root == "" {
		return "", errors.New("release archive is empty")
	}
	return filepath.Join(destination, root), nil
}

func (a *Agent) restoreAndRestart() error {
	if err := a.restoreBackup(); err != nil {
		return err
	}
	return a.restartService()
}

func (a *Agent) waitForReadiness(ctx context.Context) bool {
	timeout, interval := a.manager.ReadinessConfig()
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 3 * time.Second}
	for {
		select {
		case <-checkCtx.Done():
			return false
		case <-ticker.C:
			resp, err := client.Get(a.config.ReadinessURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return true
				}
			}
		}
	}
}

func (a *Agent) restartService() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reload := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	reload.Stdout = os.Stdout
	reload.Stderr = os.Stderr
	if err := reload.Run(); err != nil {
		return fmt.Errorf("reload systemd units: %w", err)
	}
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "velora-dns")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (a *Agent) saveState() error {
	return a.manager.SaveState()
}

func verifyChecksum(filePath, checksumPath string) error {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	var expected string
	name := filepath.Base(filePath)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			expected = fields[0]
			break
		}
	}
	if len(expected) != 64 {
		return errors.New("invalid checksum file")
	}
	expected = strings.ToLower(expected)
	if _, err := hex.DecodeString(expected); err != nil {
		return errors.New("invalid checksum file")
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}
