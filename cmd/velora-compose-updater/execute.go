package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

func (a *Agent) executeUpdate(entry *update.Entry, release update.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.Timeout)
	defer cancel()
	log := a.config.Logger.With("update_id", entry.ID)

	previousDigest := a.getCurrentDigest(ctx)

	if err := a.pullImage(ctx, release.Image); err != nil {
		log.Error("pull failed", "error", err)
		_ = a.manager.Fail(entry.ID, fmt.Errorf("pull: %w", err))
		_ = a.saveState()
		return
	}
	if err := a.manager.Transition(entry.ID, update.StateVerifying); err != nil {
		log.Error("transition failed", "error", err)
		return
	}
	_ = a.saveState()
	targetDigest, err := a.verifyImage(ctx, release.Image)
	if err != nil {
		_ = a.manager.Fail(entry.ID, fmt.Errorf("verify image: %w", err))
		_ = a.saveState()
		return
	}
	if err := a.manager.Transition(entry.ID, update.StateInstalling); err != nil {
		log.Error("transition failed", "error", err)
		return
	}

	if err := a.updateService(ctx, targetDigest); err != nil {
		log.Error("update failed", "error", err)
		_ = a.manager.Fail(entry.ID, fmt.Errorf("update: %w", err))
		_ = a.saveState()
		return
	}

	if err := a.manager.Transition(entry.ID, update.StateReadiness); err != nil {
		log.Error("transition failed", "error", err)
		return
	}

	ready := a.waitForReadiness(ctx)
	if !ready {
		log.Error("readiness failed, rolling back")
		if err := a.restorePreviousDigest(ctx, previousDigest); err != nil {
			log.Error("rollback failed", "error", err)
			_ = a.manager.Fail(entry.ID, fmt.Errorf("rollback: %w", err))
		} else if !a.waitForReadiness(context.Background()) {
			_ = a.manager.Fail(entry.ID, fmt.Errorf("rollback completed but restored service is not ready"))
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
	if a.config.VersionFile != "" {
		if err := os.WriteFile(a.config.VersionFile, []byte(release.Version+"\n"), 0640); err != nil {
			log.Error("persist installed version failed", "error", err)
		}
	}
	_ = a.saveState()
	log.Info("compose update completed")
}

func (a *Agent) getCurrentDigest(ctx context.Context) string {
	out, err := a.runCompose(ctx, "", "images", "-q", a.config.ServiceName)
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	return strings.TrimSpace(out)
}

func (a *Agent) pullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("release image is empty")
	}
	out, err := a.runCompose(ctx, image, "pull", a.config.ServiceName)
	if err != nil {
		return fmt.Errorf("pull output: %s, error: %w", out, err)
	}
	return nil
}

func (a *Agent) verifyImage(ctx context.Context, image string) (string, error) {
	if strings.TrimSpace(a.config.ComposePath) == "" {
		return "", fmt.Errorf("compose command is empty")
	}
	// A successful compose image query confirms that the pulled target resolves locally.
	out, err := a.runCompose(ctx, image, "images", "-q", a.config.ServiceName)
	if err != nil {
		return "", fmt.Errorf("target image unavailable: %s: %w", strings.TrimSpace(out), err)
	}
	digest := strings.TrimSpace(out)
	if !strings.HasPrefix(digest, "sha256:") {
		return "", fmt.Errorf("target image has no immutable digest")
	}
	return digest, nil
}

func (a *Agent) updateService(ctx context.Context, image string) error {
	out, err := a.runCompose(ctx, image, "up", "-d", "--no-deps", a.config.ServiceName)
	if err != nil {
		return fmt.Errorf("up output: %s, error: %w", out, err)
	}
	return nil
}

func (a *Agent) restorePreviousDigest(ctx context.Context, previousDigest string) error {
	if previousDigest == "" {
		return nil
	}
	out, err := a.runCompose(ctx, previousDigest, "up", "-d", "--no-deps", a.config.ServiceName)
	if err != nil {
		return fmt.Errorf("restore output: %s, error: %w", out, err)
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

func (a *Agent) saveState() error {
	return a.manager.SaveState()
}
