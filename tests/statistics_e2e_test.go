package tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	wire "github.com/miekg/dns"
)

func TestStatisticsCheckpointRestartAndReset(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	path := filepath.Join(t.TempDir(), "statistics.db")
	cfg := config.Default()
	cfg.DatabasePath = path
	cfg.DNS.Upstreams = []string{"127.0.0.1:9"}
	cfg.Management.BootstrapUsername = "admin"
	cfg.Management.BootstrapPassword = "test admin password"
	client := &http.Client{Timeout: 2 * time.Second}
	start := func() (string, string, func()) {
		cfg.DNS.Listen = []string{unusedTCPAddress(t)}
		cfg.HTTP.Listen = unusedTCPAddress(t)
		base := "http://" + cfg.HTTP.Listen
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- app.Run(ctx, cfg, filepath.Join(t.TempDir(), "config.yaml"), slog.New(slog.NewTextHandler(io.Discard, nil)), api.Version{})
		}()
		deadline := time.Now().Add(10 * time.Second)
		for {
			response, err := client.Get(base + "/ready")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					break
				}
			}
			if time.Now().After(deadline) {
				cancel()
				t.Fatalf("server did not become ready: %v", err)
			}
			time.Sleep(25 * time.Millisecond)
		}
		stop := func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("server shutdown: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("server did not stop")
			}
		}
		return base, cfg.DNS.Listen[0], stop
	}
	get := func(base, path, cookie string) []byte {
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("GET %s: %d %s", path, response.StatusCode, body)
		}
		return body
	}
	base, address, stop := start()
	message := new(wire.Msg)
	message.SetQuestion("missing.example.test.", wire.TypeA)
	if _, _, err := (&wire.Client{Net: "udp", Timeout: 2 * time.Second}).Exchange(message, address); err != nil {
		t.Fatal(err)
	}
	store, err := database.Open(context.Background(), "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deadline := time.Now().Add(8 * time.Second)
	for {
		stats, err := store.LoadStatistics(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if stats.Queries > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("periodic statistics checkpoint did not persist DNS query")
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	base, _, stop = start()
	login, err := client.Post(base+"/api/v1/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"test admin password"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != 200 || len(login.Cookies()) == 0 {
		t.Fatalf("login: %d", login.StatusCode)
	}
	cookie := login.Cookies()[0].String()
	if body := get(base, "/api/v1/stats", cookie); !strings.Contains(string(body), `"queries_total":1`) {
		t.Fatalf("statistics after restart: %s", body)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/stats/reset", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", session.Data.CSRF)
	req.AddCookie(login.Cookies()[0])
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("reset: %d", response.StatusCode)
	}
	if body := get(base, "/api/v1/stats", cookie); !strings.Contains(string(body), `"queries_total":0`) {
		t.Fatalf("statistics after reset: %s", body)
	}
	stop()
	base, _, stop = start()
	defer stop()
	if body := get(base, "/api/v1/stats", cookie); !strings.Contains(string(body), `"queries_total":0`) {
		t.Fatalf("statistics after second restart: %s", body)
	}
}
