package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolverSelectsChannelAndArchitecture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
          {"tag_name":"v2.0.0-beta.1","published_at":"2026-01-02T03:04:05Z","body":"Beta notes","assets":[
            {"name":"velora-dns-2.0.0-beta.1-linux-amd64.tar.gz","browser_download_url":"`+serverURL(r)+`/bundle","size":42},
            {"name":"CHECKSUMS.sha256","browser_download_url":"`+serverURL(r)+`/checksum"}]},
          {"tag_name":"v1.5.0","published_at":"2026-01-01T03:04:05Z","body":"Stable notes","assets":[
            {"name":"velora-dns-1.5.0-linux-amd64.tar.gz","browser_download_url":"`+serverURL(r)+`/bundle","size":21},
            {"name":"CHECKSUMS.sha256","browser_download_url":"`+serverURL(r)+`/checksum"}]}
        ]`)
	}))
	defer server.Close()

	resolver := Resolver{Repository: "owner/repo", Channel: "beta", APIBase: server.URL, GOOS: "linux", GOARCH: "amd64"}
	release, err := resolver.Resolve(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "2.0.0-beta.1" || release.Channel != "beta" || release.DownloadSize != 42 || release.Architecture != "linux/amd64" {
		t.Fatalf("release = %#v", release)
	}
}

func TestResolverRejectsBadInputsAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver Resolver
		want     string
	}{
		{"repository", Resolver{Repository: "../bad", GOOS: "linux", GOARCH: "amd64"}, "repository"},
		{"channel", Resolver{Repository: "owner/repo", Channel: "nightly", GOOS: "linux", GOARCH: "amd64"}, "channel"},
		{"architecture", Resolver{Repository: "owner/repo", GOOS: "linux", GOARCH: "386"}, "architecture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.resolver.Resolve(context.Background(), "1.0.0")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{`) }))
	defer server.Close()
	_, err := (Resolver{Repository: "owner/repo", APIBase: server.URL, GOOS: "linux", GOARCH: "amd64"}).Resolve(context.Background(), "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("malformed metadata error = %v", err)
	}
}

func TestResolverReportsNoUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[]`) }))
	defer server.Close()
	_, err := (Resolver{Repository: "owner/repo", APIBase: server.URL, GOOS: "linux", GOARCH: "amd64"}).Resolve(context.Background(), "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "no eligible") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolverSelectsHighestVersionNotPublicationOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
          {"tag_name":"v1.1.0","published_at":"2026-02-02T00:00:00Z","assets":[
            {"name":"velora-dns-1.1.0-linux-amd64.tar.gz","browser_download_url":"`+serverURL(r)+`/old"},
            {"name":"CHECKSUMS.sha256","browser_download_url":"`+serverURL(r)+`/checksums"}]},
          {"tag_name":"v1.2.0","published_at":"2026-02-01T00:00:00Z","assets":[
            {"name":"velora-dns-1.2.0-linux-amd64.tar.gz","browser_download_url":"`+serverURL(r)+`/new"},
            {"name":"CHECKSUMS.sha256","browser_download_url":"`+serverURL(r)+`/checksums"}]}
        ]`)
	}))
	defer server.Close()
	release, err := (Resolver{Repository: "owner/repo", APIBase: server.URL, GOOS: "linux", GOARCH: "amd64"}).Resolve(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.2.0" {
		t.Fatalf("selected %q, want 1.2.0", release.Version)
	}
}

func TestReleaseChannelMatchesReleaseTooling(t *testing.T) {
	tests := map[string]string{
		"1.0.0":           "stable",
		"1.0.0-rc.1":      "stable",
		"1.0.0-beta.1":    "beta",
		"1.0.0-alpha.1":   "alpha",
		"1.0.0-preview.1": "alpha",
	}
	for version, want := range tests {
		if got := releaseChannel(version); got != want {
			t.Errorf("releaseChannel(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestVersionComparisonHandlesNumericPrereleases(t *testing.T) {
	if compareVersions("2.0.0-beta.10", "2.0.0-beta.2") <= 0 {
		t.Fatal("beta.10 should be newer than beta.2")
	}
	if compareVersions("2.0.0", "2.0.0-rc.1") <= 0 {
		t.Fatal("stable should be newer than prerelease")
	}
}

func serverURL(r *http.Request) string { return "http://" + r.Host }
