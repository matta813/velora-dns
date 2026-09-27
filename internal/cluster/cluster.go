// Package cluster runs a single-writer primary/replica cluster.
//
// One primary accepts management writes. Replicas join with a one-time token,
// authenticate with a per-node credential and periodically pull a signed
// snapshot of the primary's authoritative zones, which they apply locally and
// serve. Replicas reject local zone changes, so the zone data has exactly one
// writer and cannot diverge between partitions. There is no automatic
// failover: see docs/cluster.md and ADR 0004.
package cluster

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/zones"
)

// Protocol is bumped when join or snapshot formats change incompatibly.
const Protocol = 1

const (
	RoleStandalone = "standalone"
	RolePrimary    = "primary"
	RoleReplica    = "replica"

	JoinTokenLifetime = 30 * time.Minute
	SyncInterval      = 30 * time.Second
	// A replica not seen for this long is shown as stale on the primary.
	StaleAfter = 3 * SyncInterval
	MaxMembers = 16
)

var (
	ErrInvalid       = errors.New("invalid cluster request")
	ErrState         = errors.New("not allowed in the current cluster role")
	ErrUnauthorized  = errors.New("invalid or expired credentials")
	ErrNotFound      = errors.New("cluster node not found")
	ErrConflict      = errors.New("cluster conflict")
	ErrIncompatible  = errors.New("incompatible cluster protocol")
	ErrZonesReadOnly = errors.New("zones are managed by the cluster primary on this node")
)

// State is this node's cluster configuration. The replica credential is
// never serialized.
type State struct {
	Role            string     `json:"role"`
	ClusterID       string     `json:"cluster_id"`
	NodeID          string     `json:"node_id"`
	NodeName        string     `json:"node_name"`
	AdvertisedURL   string     `json:"advertised_url"`
	PrimaryURL      string     `json:"primary_url"`
	AllowInsecure   bool       `json:"allow_insecure"`
	CreatedAt       *time.Time `json:"created_at"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastSyncError   string     `json:"last_sync_error"`
	AppliedRevision string     `json:"applied_revision"`
	Secret          string     `json:"-"`
}

// Member is a replica registered on the primary.
type Member struct {
	NodeID          string     `json:"node_id"`
	Name            string     `json:"name"`
	Address         string     `json:"address"`
	Version         string     `json:"version"`
	JoinedAt        time.Time  `json:"joined_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	AppliedRevision string     `json:"applied_revision"`
	LastError       string     `json:"last_error"`
}

type Store interface {
	LoadClusterState(context.Context) (State, bool, error)
	SaveClusterState(context.Context, State) error
	ListClusterMembers(context.Context) ([]Member, error)
	ClusterMember(ctx context.Context, nodeID string) (Member, string, error)
	SaveClusterMember(ctx context.Context, member Member, credentialHash string) error
	// TouchClusterMember records a sync report; an empty version keeps the old one.
	TouchClusterMember(ctx context.Context, nodeID string, seen time.Time, revision, lastError, version string) error
	DeleteClusterMember(ctx context.Context, nodeID string) error
	DeleteClusterMembers(context.Context) error
	SaveJoinToken(ctx context.Context, hash string, expires time.Time) error
	// ConsumeJoinToken deletes the token and reports whether it was valid at now.
	ConsumeJoinToken(ctx context.Context, hash string, now time.Time) (bool, error)
}

// Zones is the local zone service the cluster reads and, on replicas, writes.
type Zones interface {
	List() []zones.Zone
	Create(context.Context, zones.Zone) (zones.Zone, error)
	Update(context.Context, int64, uint32, zones.Zone) (zones.Zone, error)
	Delete(context.Context, int64, uint32) error
}

// Snapshot is what a replica pulls.
type Snapshot struct {
	Protocol    int              `json:"protocol"`
	ClusterID   string           `json:"cluster_id"`
	Revision    string           `json:"revision"`
	GeneratedAt time.Time        `json:"generated_at"`
	Zones       []SnapshotZone   `json:"zones"`
	Primary     SnapshotIdentity `json:"primary"`
}

type SnapshotIdentity struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SnapshotZone carries only replicated content, never local IDs.
type SnapshotZone struct {
	Name      string           `json:"name"`
	PrimaryNS string           `json:"primary_ns"`
	Contact   string           `json:"contact"`
	Records   []SnapshotRecord `json:"records"`
}

type SnapshotRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      uint32 `json:"ttl"`
	Value    string `json:"value"`
	Priority uint16 `json:"priority"`
}

// JoinRequest is sent by a joining node to the primary.
type JoinRequest struct {
	Token    string `json:"token"`
	Protocol int    `json:"protocol"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

// JoinResponse returns the replica's credential; it is shown only once.
type JoinResponse struct {
	ClusterID      string `json:"cluster_id"`
	NodeSecret     string `json:"node_secret"`
	PrimaryName    string `json:"primary_name"`
	PrimaryVersion string `json:"primary_version"`
}

func randomToken(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func newID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// signatureKey is derived from the credential; the primary only stores the
// hash, which is exactly this key.
func signatureKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return []byte(hex.EncodeToString(sum[:]))
}

func sign(key []byte, body []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateURL accepts an http(s) base URL without credentials, query or
// fragment. Plain HTTP must be allowed explicitly.
func ValidateURL(raw string, allowInsecure bool) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || len(raw) > 512 {
		return "", fmt.Errorf("%w: enter a base URL such as https://dns1.example.lan:8443", ErrInvalid)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowInsecure {
			return "", fmt.Errorf("%w: plain HTTP sends the join token and node credential unencrypted; use HTTPS or explicitly allow HTTP on a trusted network", ErrInvalid)
		}
	default:
		return "", fmt.Errorf("%w: the URL must start with https:// or http://", ErrInvalid)
	}
	return u.Scheme + "://" + u.Host, nil
}

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n\x00") {
		return "", fmt.Errorf("%w: node name must be 1–64 characters on one line", ErrInvalid)
	}
	return name, nil
}

// BuildSnapshot captures the primary's authoritative (non-secondary) zones.
// The revision is a content hash, so it only changes when data changes.
func BuildSnapshot(all []zones.Zone, clusterID string, primary SnapshotIdentity, now time.Time) Snapshot {
	out := Snapshot{Protocol: Protocol, ClusterID: clusterID, GeneratedAt: now.UTC(), Primary: primary, Zones: []SnapshotZone{}}
	for _, zone := range all {
		if zone.ZoneType == "secondary" {
			continue
		}
		item := SnapshotZone{Name: zone.Name, PrimaryNS: zone.PrimaryNS, Contact: zone.Contact, Records: []SnapshotRecord{}}
		for _, record := range zone.Records {
			item.Records = append(item.Records, SnapshotRecord{Name: record.Name, Type: record.Type, TTL: record.TTL, Value: record.Value, Priority: record.Priority})
		}
		sort.Slice(item.Records, func(i, j int) bool {
			a, b := item.Records[i], item.Records[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			if a.Type != b.Type {
				return a.Type < b.Type
			}
			return a.Value < b.Value
		})
		out.Zones = append(out.Zones, item)
	}
	sort.Slice(out.Zones, func(i, j int) bool { return out.Zones[i].Name < out.Zones[j].Name })
	content, _ := json.Marshal(out.Zones)
	sum := sha256.Sum256(content)
	out.Revision = hex.EncodeToString(sum[:8])
	return out
}
