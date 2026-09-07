package filtering

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseHosts(t *testing.T) {
	got := ParseHosts([]byte("# comment\n0.0.0.0 ads.example\ntracker.example # note\ninvalid_thing\n0.0.0.0 ads.example\n"))
	if strings.Join(got, ",") != "ads.example,tracker.example" {
		t.Fatalf("%v", got)
	}
}
func TestFetchHostsRejectsPrivateResolution(t *testing.T) {
	_, err := FetchHosts(context.Background(), "https://lists.example/a", nil, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	if err == nil {
		t.Fatal("private source accepted")
	}
}
func TestFetchHostsBoundedAndNoRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.example", http.StatusFound)
	}))
	defer target.Close()
	_, err := FetchHosts(context.Background(), target.URL, nil, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	if err == nil {
		t.Fatal("redirect accepted")
	}
}
