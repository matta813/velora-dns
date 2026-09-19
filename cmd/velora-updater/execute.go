package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

func (a *Agent) executeUpdate(entry *update.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.Timeout)
	defer cancel()
	log := a.config.Logger.With("update_id", entry.ID)

	if err := a.manager.Transition(entry.ID, update.StateVerifying); err != nil {
		log.Error("transition to verifying failed", "error", err)
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

	if err := a.installRelease(ctx); err != nil {
		log.Error("install failed", "error", err)
		if rbErr := a.restoreBackup(); rbErr != nil {
			log.Error("restore also failed", "error", rbErr)
			_ = a.manager.Fail(entry.ID, fmt.Errorf("install failed: %v, restore failed: %v", err, rbErr))
		} else {
			_ = a.manager.Rollback(entry.ID)
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
		if rbErr := a.restoreBackup(); rbErr != nil {
			log.Error("restore also failed", "error", rbErr)
			_ = a.manager.Fail(entry.ID, fmt.Errorf("readiness failed, restore failed: %v", rbErr))
		} else {
			_ = a.manager.Rollback(entry.ID)
		}
		_ = a.saveState()
		return
	}

	if err := a.restartService(); err != nil {
		log.Error("restart failed", "error", err)
		_ = a.manager.Fail(entry.ID, fmt.Errorf("restart: %w", err))
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
	return nil
}

func (a *Agent) installRelease(ctx context.Context) error {
	bundlePath := os.Getenv("VELORA_UPDATE_BUNDLE_PATH")
	if bundlePath == "" {
		return fmt.Errorf("VELORA_UPDATE_BUNDLE_PATH not set")
	}

	checksumPath := bundlePath + ".sha256"
	if _, err := os.Stat(checksumPath); err == nil {
		if err := verifyChecksum(bundlePath, checksumPath); err != nil {
			return fmt.Errorf("checksum verification: %w", err)
		}
	}

	binSrc := filepath.Join(bundlePath, "velora-dns")
	if _, err := os.Stat(binSrc); err == nil {
		if err := copyFile(binSrc, a.config.BinaryPath, 0755); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
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
	expected := string(data[:64])

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
