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
	got, err := ParseHosts([]byte("# comment\n0.0.0.0 ads.example\ntracker.example # note\n0.0.0.0 ads.example\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "ads.example,tracker.example" {
		t.Fatalf("%v", got)
	}
}

func TestParseHostsRejectsUnderscore(t *testing.T) {
	if _, err := ParseHosts([]byte("0.0.0.0 invalid_thing\n")); err == nil {
		t.Fatal("expected error for underscore domain")
	}
}

func TestParseHostsWildcard(t *testing.T) {
	got, err := ParseHosts([]byte("0.0.0.0 *.ads.example\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "*.ads.example" {
		t.Fatalf("%v", got)
	}
}

func TestParseHostsRejectsMissingAlias(t *testing.T) {
	if _, err := ParseHosts([]byte("0.0.0.0\n")); err == nil {
		t.Fatal("expected error for hosts line without alias")
	}
}

func TestParseHostsRejectsEmpty(t *testing.T) {
	if _, err := ParseHosts([]byte("# only comments\n")); err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestFetchHostsRejectsPrivateResolution(t *testing.T) {
	_, err := fetchHosts(context.Background(), "https://lists.example/a", func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) { return nil, nil })
	if err == nil {
		t.Fatal("private source accepted")
	}
}

func TestFetchHostsBoundedAndNoRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.example", http.StatusFound)
	}))
	defer target.Close()
	_, err := fetchHosts(context.Background(), target.URL, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) { return nil, nil })
	if err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestValidateSourceURL(t *testing.T) {
	for _, tc := range []struct {
		raw string
		ok  bool
	}{{"https://lists.example/a", true}, {"http://lists.example/a", true}, {"file:///etc/passwd", false}, {"https://user:pass@lists.example/a", false}, {"https://127.0.0.1/a", false}, {"https://lists.example:8443/a", false}, {"not a url", false}} {
		err := ValidateSourceURL(tc.raw)
		if (err == nil) != tc.ok {
			t.Errorf("%q: ok=%v err=%v", tc.raw, tc.ok, err)
		}
	}
}
