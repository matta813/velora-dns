package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var ErrNoEligibleRelease = errors.New("no eligible release found")

// Release describes a trusted published release selected for this host.
type Release struct {
	Version      string    `json:"version"`
	Channel      string    `json:"channel"`
	PublishedAt  time.Time `json:"release_date"`
	Notes        string    `json:"release_notes"`
	Architecture string    `json:"architecture"`
	DownloadSize int64     `json:"download_size,omitempty"`
	ArtifactURL  string    `json:"-"`
	ChecksumURL  string    `json:"-"`
	Image        string    `json:"-"`
}

// CheckResult is returned by updater agents without exposing trusted asset URLs.
type CheckResult struct {
	Installed       string    `json:"installed_version"`
	Latest          string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	Channel         string    `json:"channel"`
	ReleaseDate     time.Time `json:"release_date,omitempty"`
	ReleaseNotes    string    `json:"release_notes,omitempty"`
	Architecture    string    `json:"architecture"`
	DownloadSize    int64     `json:"download_size,omitempty"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type githubRelease struct {
	Tag         string        `json:"tag_name"`
	Body        string        `json:"body"`
	Draft       bool          `json:"draft"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

// Resolver discovers releases from one configured GitHub repository.
type Resolver struct {
	Repository string
	Channel    string
	Client     *http.Client
	APIBase    string
	GOOS       string
	GOARCH     string
}

func (r Resolver) Resolve(ctx context.Context, installed string) (Release, error) {
	if !validRepository(r.Repository) {
		return Release{}, errors.New("invalid release repository")
	}
	channel := r.Channel
	if channel == "" {
		channel = "stable"
	}
	if channel != "stable" && channel != "beta" && channel != "alpha" {
		return Release{}, fmt.Errorf("unsupported release channel %q", channel)
	}
	goos, goarch := r.GOOS, r.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if goos != "linux" || (goarch != "amd64" && goarch != "arm64") {
		return Release{}, fmt.Errorf("unsupported architecture %s/%s", goos, goarch)
	}
	base := strings.TrimRight(r.APIBase, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+r.Repository+"/releases?per_page=30", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "velora-dns-updater")
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("discover releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("discover releases: HTTP %d", response.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&releases); err != nil {
		return Release{}, fmt.Errorf("decode release metadata: %w", err)
	}
	var selected Release
	for _, candidate := range releases {
		if candidate.Draft {
			continue
		}
		version := strings.TrimPrefix(candidate.Tag, "v")
		if !validVersion(version) || releaseChannel(version) != channel || compareVersions(version, strings.TrimPrefix(installed, "v")) <= 0 {
			continue
		}
		name := fmt.Sprintf("velora-dns-%s-%s-%s.tar.gz", version, goos, goarch)
		var artifact, checksum githubAsset
		for _, asset := range candidate.Assets {
			switch asset.Name {
			case name:
				artifact = asset
			case "CHECKSUMS.sha256":
				checksum = asset
			}
		}
		if artifact.URL == "" || checksum.URL == "" {
			continue
		}
		trustedPrefix := "https://github.com/" + r.Repository + "/releases/download/"
		if base == "https://api.github.com" && (!strings.HasPrefix(artifact.URL, trustedPrefix) || !strings.HasPrefix(checksum.URL, trustedPrefix)) {
			return Release{}, errors.New("release contains an untrusted asset URL")
		}
		resolved := Release{
			Version: version, Channel: channel, PublishedAt: candidate.PublishedAt,
			Notes: candidate.Body, Architecture: goos + "/" + goarch,
			DownloadSize: artifact.Size, ArtifactURL: artifact.URL, ChecksumURL: checksum.URL,
			Image: "ghcr.io/" + strings.ToLower(r.Repository) + ":" + version,
		}
		if selected.Version == "" || compareVersions(resolved.Version, selected.Version) > 0 {
			selected = resolved
		}
	}
	if selected.Version != "" {
		return selected, nil
	}
	return Release{}, ErrNoEligibleRelease
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, "\\?#%") || strings.Contains(part, "..") {
			return false
		}
	}
	return true
}

func releaseChannel(version string) string {
	lower := strings.ToLower(version)
	if !strings.Contains(lower, "-") || strings.Contains(lower, "-rc.") {
		return "stable"
	}
	if strings.Contains(lower, "-alpha.") {
		return "alpha"
	}
	if strings.Contains(lower, "-beta.") {
		return "beta"
	}
	return "alpha"
}

func validVersion(version string) bool {
	main := strings.SplitN(version, "-", 2)[0]
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.ParseUint(part, 10, 64); err != nil {
			return false
		}
	}
	return true
}

func compareVersions(a, b string) int {
	if !validVersion(b) {
		return 1
	}
	parse := func(version string) ([3]uint64, string) {
		pieces := strings.SplitN(version, "-", 2)
		parts := strings.Split(pieces[0], ".")
		var numbers [3]uint64
		for i := range numbers {
			numbers[i], _ = strconv.ParseUint(parts[i], 10, 64)
		}
		pre := ""
		if len(pieces) == 2 {
			pre = pieces[1]
		}
		return numbers, pre
	}
	an, ap := parse(a)
	bn, bp := parse(b)
	for i := range an {
		if an[i] > bn[i] {
			return 1
		}
		if an[i] < bn[i] {
			return -1
		}
	}
	if ap == bp {
		return 0
	}
	if ap == "" {
		return 1
	}
	if bp == "" {
		return -1
	}
	aParts, bParts := strings.Split(ap, "."), strings.Split(bp, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		an, aErr := strconv.ParseUint(aParts[i], 10, 64)
		bn, bErr := strconv.ParseUint(bParts[i], 10, 64)
		if aErr == nil && bErr == nil {
			if an > bn {
				return 1
			}
			if an < bn {
				return -1
			}
			continue
		}
		if aErr == nil {
			return -1
		}
		if bErr == nil {
			return 1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
		if aParts[i] < bParts[i] {
			return -1
		}
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return -1
}
