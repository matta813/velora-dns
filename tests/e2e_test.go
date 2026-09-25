package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

func unusedTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func TestManagementDNSWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	dnsAddress, httpAddress := unusedTCPAddress(t), unusedTCPAddress(t)
	cfg := config.Default()
	cfg.DNS.Listen = []string{dnsAddress}
	cfg.DNS.Upstreams = []string{"127.0.0.1:9"}
	cfg.HTTP.Listen = httpAddress
	cfg.DatabasePath = filepath.Join(t.TempDir(), "velora.db")
	cfg.QueryLog.Enabled = true
	cfg.Management.BootstrapUsername = "admin"
	cfg.Management.BootstrapPassword = "test admin password"
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, cfg, configPath, slog.New(slog.NewTextHandler(t.Output(), nil)), api.Version{Version: "e2e"})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	base := "http://" + httpAddress
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := client.Get(base + "/ready")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case err := <-done:
			t.Fatalf("server stopped before readiness: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	var cookie, csrf string
	request := func(method, path, body, revision string) (int, http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if csrf != "" && method != http.MethodGet {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if revision != "" {
			req.Header.Set("If-Match", revision)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, response.Header, payload
	}
	status, headers, payload := request(http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"test admin password"}`, "")
	if status != 200 {
		t.Fatalf("login: %d %s", status, payload)
	}
	for _, value := range headers.Values("Set-Cookie") {
		if strings.HasPrefix(value, "velora_session=") {
			cookie = strings.SplitN(value, ";", 2)[0]
		}
	}
	var login struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &login); err != nil {
		t.Fatal(err)
	}
	csrf = login.Data.CSRFToken
	if cookie == "" || csrf == "" {
		t.Fatalf("login omitted session or CSRF token: %s", payload)
	}

	status, _, payload = request(http.MethodPost, "/api/v1/zones", `{"name":"home.test"}`, "")
	if status != 201 {
		t.Fatalf("create zone: %d %s", status, payload)
	}
	var zone struct {
		Data zones.Zone `json:"data"`
	}
	if err := json.Unmarshal(payload, &zone); err != nil {
		t.Fatal(err)
	}
	zonePath := fmt.Sprintf("/api/v1/zones/%d", zone.Data.ID)
	status, _, payload = request(http.MethodPost, zonePath+"/records", `{"name":"host","type":"A","ttl":60,"value":"192.0.2.10"}`, `"1"`)
	if status != 201 {
		t.Fatalf("create record: %d %s", status, payload)
	}
	if err := json.Unmarshal(payload, &zone); err != nil {
		t.Fatal(err)
	}
	if len(zone.Data.Records) != 1 {
		t.Fatalf("record not returned: %s", payload)
	}
	recordPath := fmt.Sprintf("%s/records/%d", zonePath, zone.Data.Records[0].ID)
	query := func(want string) {
		t.Helper()
		message := new(wire.Msg)
		message.SetQuestion("host.home.test.", wire.TypeA)
		answer, _, err := (&wire.Client{Net: "udp", Timeout: 2 * time.Second}).Exchange(message, dnsAddress)
		if err != nil {
			t.Fatal(err)
		}
		if len(answer.Answer) != 1 || answer.Answer[0].(*wire.A).A.String() != want {
			t.Fatalf("DNS answer after API change: %v", answer)
		}
	}
	query("192.0.2.10")
	status, _, payload = request(http.MethodPut, recordPath, `{"name":"host","type":"A","ttl":60,"value":"192.0.2.11"}`, `"2"`)
	if status != 200 {
		t.Fatalf("update record: %d %s", status, payload)
	}
	query("192.0.2.11")
	status, _, payload = request(http.MethodPut, "/api/v1/config", `{"dns":{"allowed_clients":["invalid-cidr"]}}`, "")
	if status != 400 {
		t.Fatalf("invalid config accepted: %d %s", status, payload)
	}
	status, _, payload = request(http.MethodDelete, recordPath, "", `"3"`)
	if status != 200 {
		t.Fatalf("delete record: %d %s", status, payload)
	}
	message := new(wire.Msg)
	message.SetQuestion("host.home.test.", wire.TypeA)
	answer, _, err := (&wire.Client{Net: "udp", Timeout: 2 * time.Second}).Exchange(message, dnsAddress)
	if err != nil || answer.Rcode != wire.RcodeNameError {
		t.Fatalf("record still resolves after delete: %v (%v)", answer, err)
	}
	status, _, payload = request(http.MethodGet, "/api/v1/stats", "", "")
	if status != 200 || !bytes.Contains(payload, []byte(`"queries_total":`)) {
		t.Fatalf("statistics unavailable: %d %s", status, payload)
	}
	historyDeadline := time.Now().Add(5 * time.Second)
	for {
		status, _, payload = request(http.MethodGet, "/api/v1/queries?domain=host.home.test.", "", "")
		if status == 200 && bytes.Contains(payload, []byte(`"domain":"host.home.test."`)) {
			break
		}
		if time.Now().After(historyDeadline) {
			t.Fatalf("DNS query did not reach history: %d %s", status, payload)
		}
		time.Sleep(25 * time.Millisecond)
	}
	status, _, payload = request(http.MethodPost, "/api/v1/users", `{"username":"viewer","password":"test viewer password","role":"viewer"}`, "")
	if status != 201 {
		t.Fatalf("create viewer: %d %s", status, payload)
	}
	status, headers, payload = request(http.MethodPost, "/api/v1/auth/login", `{"username":"viewer","password":"test viewer password"}`, "")
	if status != 200 {
		t.Fatalf("viewer login: %d %s", status, payload)
	}
	for _, value := range headers.Values("Set-Cookie") {
		if strings.HasPrefix(value, "velora_session=") {
			cookie = strings.SplitN(value, ";", 2)[0]
		}
	}
	if err := json.Unmarshal(payload, &login); err != nil {
		t.Fatal(err)
	}
	csrf = login.Data.CSRFToken
	status, _, payload = request(http.MethodDelete, zonePath, "", `"4"`)
	if status != 403 {
		t.Fatalf("viewer changed zone: %d %s", status, payload)
	}
}
