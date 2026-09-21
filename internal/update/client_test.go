package update

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestClientUsesUpdaterUnixSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "updater.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Status{State: StateIdle, Installed: "1.0.0"})
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) })
	mux.HandleFunc("GET /check", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(CheckResult{Installed: "1.0.0", Latest: "1.1.0", UpdateAvailable: true})
	})
	mux.HandleFunc("POST /update", func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["action"] != "update" {
			t.Errorf("request = %#v", request)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(RequestResponse{Status: "accepted", Version: "1.1.0"})
	})
	server := &http.Server{Handler: mux}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()

	client := Client{SocketPath: socket}
	status, err := client.Status(context.Background())
	if err != nil || status.Installed != "1.0.0" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	check, err := client.Check(context.Background())
	if err != nil || !check.UpdateAvailable {
		t.Fatalf("check = %#v, %v", check, err)
	}
	history, err := client.History(context.Background())
	if err != nil || history == nil {
		t.Fatalf("history = %#v, %v", history, err)
	}
	response, err := client.Request(context.Background())
	if err != nil || response.Version != "1.1.0" {
		t.Fatalf("response = %#v, %v", response, err)
	}
	_ = os.Remove(socket)
}
