package cluster

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/zones"
)

type Options struct {
	Version string
	// Client talks to other nodes; tests replace it.
	Client *http.Client
	Now    func() time.Time
}

type Service struct {
	store   Store
	zones   Zones
	version string
	client  *http.Client
	now     func() time.Time

	mu    sync.Mutex
	state State
	// syncNow wakes the replica loop for an immediate sync.
	syncNow chan struct{}
	// onSync is told when a replica's sync starts failing or recovers.
	onSync func(failed bool, message string)
}

// SetSyncObserver registers fn for sync failures and recoveries; call it
// before RunReplica starts.
func (s *Service) SetSyncObserver(fn func(failed bool, message string)) { s.onSync = fn }

// Overview is what the management UI shows.
type Overview struct {
	State           State        `json:"state"`
	Revision        string       `json:"revision"`
	Members         []MemberView `json:"members"`
	ZonesReadOnly   bool         `json:"zones_read_only"`
	Protocol        int          `json:"protocol"`
	Version         string       `json:"version"`
	ReplicatedZones int          `json:"replicated_zones"`
}

type MemberView struct {
	Member
	// Status is in_sync, behind, stale, error or never_synced.
	Status string `json:"status"`
}

func NewService(ctx context.Context, store Store, local Zones, options Options) (*Service, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		options.Client = &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	s := &Service{store: store, zones: local, version: options.Version, client: options.Client, now: options.Now, syncNow: make(chan struct{}, 1)}
	state, found, err := store.LoadClusterState(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		state = State{Role: RoleStandalone, NodeID: id}
		if err := store.SaveClusterState(ctx, state); err != nil {
			return nil, err
		}
	}
	s.state = state
	return s, nil
}

func (s *Service) current() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Current returns this node's cluster state.
func (s *Service) Current() State { return s.current() }

func (s *Service) save(ctx context.Context, state State) error {
	if err := s.store.SaveClusterState(ctx, state); err != nil {
		return err
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return nil
}

// ZonesReadOnly reports whether local zone changes must be rejected.
func (s *Service) ZonesReadOnly() bool { return s.current().Role == RoleReplica }

func (s *Service) snapshot() Snapshot {
	state := s.current()
	return BuildSnapshot(s.zones.List(), state.ClusterID, SnapshotIdentity{NodeID: state.NodeID, Name: state.NodeName, Version: s.version}, s.now())
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	state := s.current()
	out := Overview{State: state, Members: []MemberView{}, ZonesReadOnly: state.Role == RoleReplica, Protocol: Protocol, Version: s.version}
	switch state.Role {
	case RolePrimary:
		snapshot := s.snapshot()
		out.Revision, out.ReplicatedZones = snapshot.Revision, len(snapshot.Zones)
		members, err := s.store.ListClusterMembers(ctx)
		if err != nil {
			return Overview{}, err
		}
		now := s.now()
		for _, member := range members {
			view := MemberView{Member: member}
			switch {
			case member.LastSeenAt == nil:
				view.Status = "never_synced"
			case now.Sub(*member.LastSeenAt) > StaleAfter:
				view.Status = "stale"
			case member.LastError != "":
				view.Status = "error"
			case member.AppliedRevision == snapshot.Revision:
				view.Status = "in_sync"
			default:
				view.Status = "behind"
			}
			out.Members = append(out.Members, view)
		}
	case RoleReplica:
		out.Revision = state.AppliedRevision
		for _, zone := range s.zones.List() {
			if zone.ZoneType != "secondary" {
				out.ReplicatedZones++
			}
		}
	}
	return out, nil
}

// CheckURL fetches /api/v1/cluster/peer/info from base and returns it.
func (s *Service) fetchInfo(ctx context.Context, base string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/cluster/peer/info", nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %s", base, friendlyNetError(err))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered HTTP %d%s", base, response.StatusCode, hostHint(response))
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body); err != nil || body.Data == nil {
		return nil, fmt.Errorf("%s is not a Velora DNS management address", base)
	}
	return body.Data, nil
}

func friendlyNetError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "connection refused"):
		return "connection refused (is the web interface listening on that address and port?)"
	case strings.Contains(message, "no such host"):
		return "the host name does not resolve"
	case strings.Contains(message, "certificate"):
		return "TLS certificate is not trusted: " + message
	case strings.Contains(message, "deadline exceeded") || strings.Contains(message, "timeout"):
		return "timed out"
	}
	return message
}

func hostHint(response *http.Response) string {
	if response.StatusCode == http.StatusForbidden {
		return " — add this host name to http.allowed_hosts on that node"
	}
	return ""
}

// Create turns a standalone node into the primary of a new cluster. Unless
// skipCheck is set, the advertised URL must reach this node.
func (s *Service) Create(ctx context.Context, name, advertisedURL string, allowInsecure, skipCheck bool) (State, error) {
	state := s.current()
	if state.Role != RoleStandalone {
		return State{}, fmt.Errorf("%w: this node is already a cluster %s", ErrState, state.Role)
	}
	name, err := validName(name)
	if err != nil {
		return State{}, err
	}
	base, err := ValidateURL(advertisedURL, allowInsecure)
	if err != nil {
		return State{}, err
	}
	clusterID, err := newID()
	if err != nil {
		return State{}, err
	}
	now := s.now().UTC()
	next := state
	next.Role, next.ClusterID, next.NodeName, next.AdvertisedURL, next.AllowInsecure, next.CreatedAt = RolePrimary, clusterID, name, base, allowInsecure, &now
	if err := s.save(ctx, next); err != nil {
		return State{}, err
	}
	if !skipCheck {
		info, err := s.fetchInfo(ctx, base)
		if err == nil && info["cluster_id"] != clusterID {
			err = fmt.Errorf("%s answered for a different Velora node", base)
		}
		if err != nil {
			_ = s.save(ctx, state)
			return State{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	return next, nil
}

// IssueJoinToken creates a one-time token for a new replica.
func (s *Service) IssueJoinToken(ctx context.Context) (string, time.Time, error) {
	if s.current().Role != RolePrimary {
		return "", time.Time{}, fmt.Errorf("%w: only the primary can add nodes", ErrState)
	}
	members, err := s.store.ListClusterMembers(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if len(members) >= MaxMembers {
		return "", time.Time{}, fmt.Errorf("%w: at most %d replicas", ErrInvalid, MaxMembers)
	}
	token, err := randomToken("vjt_")
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(JoinTokenLifetime).UTC()
	if err := s.store.SaveJoinToken(ctx, hashSecret(token), expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// AcceptJoin runs on the primary when a replica presents a join token.
func (s *Service) AcceptJoin(ctx context.Context, request JoinRequest, address string) (JoinResponse, error) {
	state := s.current()
	if state.Role != RolePrimary {
		return JoinResponse{}, fmt.Errorf("%w: this node is not a cluster primary", ErrState)
	}
	if request.Protocol != Protocol {
		return JoinResponse{}, fmt.Errorf("%w: the joining node speaks cluster protocol %d, this primary speaks %d; update both to the same Velora version", ErrIncompatible, request.Protocol, Protocol)
	}
	name, err := validName(request.Name)
	if err != nil {
		return JoinResponse{}, err
	}
	if len(request.NodeID) != 24 || strings.Trim(request.NodeID, "0123456789abcdef") != "" {
		return JoinResponse{}, fmt.Errorf("%w: invalid node ID", ErrInvalid)
	}
	if request.NodeID == state.NodeID {
		return JoinResponse{}, fmt.Errorf("%w: a node cannot join its own cluster", ErrConflict)
	}
	valid, err := s.store.ConsumeJoinToken(ctx, hashSecret(request.Token), s.now())
	if err != nil {
		return JoinResponse{}, err
	}
	if !valid {
		return JoinResponse{}, fmt.Errorf("%w: the join token is invalid, already used or expired; create a new one on the primary", ErrUnauthorized)
	}
	if _, _, err := s.store.ClusterMember(ctx, request.NodeID); err == nil {
		return JoinResponse{}, fmt.Errorf("%w: a node with this ID is already a member; remove it on the primary first", ErrConflict)
	}
	secret, err := randomToken("vns_")
	if err != nil {
		return JoinResponse{}, err
	}
	member := Member{NodeID: request.NodeID, Name: name, Address: address, Version: request.Version, JoinedAt: s.now().UTC()}
	if err := s.store.SaveClusterMember(ctx, member, hashSecret(secret)); err != nil {
		return JoinResponse{}, err
	}
	return JoinResponse{ClusterID: state.ClusterID, NodeSecret: secret, PrimaryName: state.NodeName, PrimaryVersion: s.version}, nil
}

// Authenticate checks a replica's "VeloraNode <id>:<secret>" credential and
// returns the member and the signature key for responses.
func (s *Service) Authenticate(ctx context.Context, header string) (Member, []byte, error) {
	if s.current().Role != RolePrimary {
		return Member{}, nil, ErrUnauthorized
	}
	value, ok := strings.CutPrefix(header, "VeloraNode ")
	nodeID, secret, found := strings.Cut(value, ":")
	if !ok || !found {
		return Member{}, nil, ErrUnauthorized
	}
	member, hash, err := s.store.ClusterMember(ctx, nodeID)
	if err != nil || !hmacEqual(hash, hashSecret(secret)) {
		return Member{}, nil, ErrUnauthorized
	}
	return member, []byte(hash), nil
}

func hmacEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ServeSnapshot records the replica's report and returns the signed snapshot.
func (s *Service) ServeSnapshot(ctx context.Context, member Member, key []byte, appliedRevision, lastError, version string) ([]byte, string, error) {
	if len(lastError) > 300 {
		lastError = lastError[:300]
	}
	if len(appliedRevision) > 64 {
		appliedRevision = ""
	}
	if len(version) > 64 {
		version = ""
	}
	if err := s.store.TouchClusterMember(ctx, member.NodeID, s.now().UTC(), appliedRevision, lastError, version); err != nil {
		return nil, "", err
	}
	body, err := json.Marshal(s.snapshot())
	if err != nil {
		return nil, "", err
	}
	return body, sign(key, body), nil
}

// RemoveMember deletes a replica on the primary. Its next sync fails and it
// stops replicating; its local copy of the zones stays in place.
func (s *Service) RemoveMember(ctx context.Context, nodeID string) error {
	state := s.current()
	if state.Role != RolePrimary {
		return fmt.Errorf("%w: only the primary manages members", ErrState)
	}
	if nodeID == state.NodeID {
		return fmt.Errorf("%w: the primary cannot remove itself; dissolve the cluster instead", ErrConflict)
	}
	if _, _, err := s.store.ClusterMember(ctx, nodeID); err != nil {
		return ErrNotFound
	}
	return s.store.DeleteClusterMember(ctx, nodeID)
}

// Dissolve returns the primary to standalone and revokes every replica.
func (s *Service) Dissolve(ctx context.Context) error {
	state := s.current()
	if state.Role != RolePrimary {
		return fmt.Errorf("%w: only the primary can dissolve the cluster", ErrState)
	}
	if err := s.store.DeleteClusterMembers(ctx); err != nil {
		return err
	}
	return s.save(ctx, State{Role: RoleStandalone, NodeID: state.NodeID})
}

// Join makes this standalone node a replica of the primary at primaryURL.
// Local zones are replaced by the primary's on the first sync.
func (s *Service) Join(ctx context.Context, primaryURL, token, name string, allowInsecure bool) (State, error) {
	state := s.current()
	if state.Role != RoleStandalone {
		return State{}, fmt.Errorf("%w: leave the current cluster first", ErrState)
	}
	name, err := validName(name)
	if err != nil {
		return State{}, err
	}
	base, err := ValidateURL(primaryURL, allowInsecure)
	if err != nil {
		return State{}, err
	}
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "vjt_") || len(token) > 100 {
		return State{}, fmt.Errorf("%w: paste the join token created on the primary (it starts with vjt_)", ErrInvalid)
	}
	info, err := s.fetchInfo(ctx, base)
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if info["role"] != RolePrimary {
		return State{}, fmt.Errorf("%w: %s is not a cluster primary", ErrInvalid, base)
	}
	if protocol, _ := info["protocol"].(float64); int(protocol) != Protocol {
		return State{}, fmt.Errorf("%w: the primary speaks cluster protocol %v, this node speaks %d; run the same Velora version on both", ErrIncompatible, info["protocol"], Protocol)
	}
	body, _ := json.Marshal(JoinRequest{Token: token, Protocol: Protocol, NodeID: state.NodeID, Name: name, Version: s.version})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/cluster/peer/join", bytes.NewReader(body))
	if err != nil {
		return State{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return State{}, fmt.Errorf("%w: cannot reach %s: %s", ErrInvalid, base, friendlyNetError(err))
	}
	defer func() { _ = response.Body.Close() }()
	var reply struct {
		Data  JoinResponse `json:"data"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&reply)
	if response.StatusCode != http.StatusOK {
		message := reply.Error.Message
		if message == "" {
			message = fmt.Sprintf("HTTP %d%s", response.StatusCode, hostHint(response))
		}
		return State{}, fmt.Errorf("%w: the primary rejected the join: %s", ErrInvalid, message)
	}
	if reply.Data.NodeSecret == "" || reply.Data.ClusterID == "" {
		return State{}, fmt.Errorf("%w: the primary sent an incomplete answer", ErrInvalid)
	}
	now := s.now().UTC()
	next := State{Role: RoleReplica, ClusterID: reply.Data.ClusterID, NodeID: state.NodeID, NodeName: name, PrimaryURL: base, AllowInsecure: allowInsecure, CreatedAt: &now, Secret: reply.Data.NodeSecret}
	if err := s.save(ctx, next); err != nil {
		return State{}, err
	}
	s.TriggerSync()
	return next, nil
}

// Leave returns a replica to standalone. It tells the primary when possible;
// local zones stay and become editable.
func (s *Service) Leave(ctx context.Context) error {
	state := s.current()
	if state.Role != RoleReplica {
		return fmt.Errorf("%w: this node is not a replica", ErrState)
	}
	if req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.PrimaryURL+"/api/v1/cluster/peer/leave", nil); err == nil {
		req.Header.Set("Authorization", "VeloraNode "+state.NodeID+":"+state.Secret)
		if response, err := s.client.Do(req); err == nil {
			_ = response.Body.Close()
		}
	}
	return s.save(ctx, State{Role: RoleStandalone, NodeID: state.NodeID})
}

// DepartMember is called on the primary when a replica leaves.
func (s *Service) DepartMember(ctx context.Context, member Member) error {
	return s.store.DeleteClusterMember(ctx, member.NodeID)
}

func (s *Service) TriggerSync() {
	select {
	case s.syncNow <- struct{}{}:
	default:
	}
}

// RunReplica syncs periodically while this node is a replica.
func (s *Service) RunReplica(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-s.syncNow:
		}
		if s.current().Role == RoleReplica {
			_ = s.SyncOnce(ctx)
		}
		timer.Reset(SyncInterval)
	}
}

// SyncOnce pulls and applies the primary's snapshot and records the outcome.
func (s *Service) SyncOnce(ctx context.Context) error {
	state := s.current()
	if state.Role != RoleReplica {
		return fmt.Errorf("%w: this node is not a replica", ErrState)
	}
	err := s.pullAndApply(ctx, state)
	state = s.current()
	if state.Role != RoleReplica {
		return err
	}
	now := s.now().UTC()
	previous := state.LastSyncError
	state.LastSyncAt = &now
	state.LastSyncError = ""
	if err != nil {
		state.LastSyncError = err.Error()
	}
	if s.onSync != nil && (previous == "") != (state.LastSyncError == "") {
		s.onSync(state.LastSyncError != "", state.LastSyncError)
	}
	if saveErr := s.save(ctx, state); saveErr != nil {
		return errors.Join(err, saveErr)
	}
	return err
}

func (s *Service) pullAndApply(ctx context.Context, state State) error {
	query := url.Values{"applied": {state.AppliedRevision}, "version": {s.version}}
	if state.LastSyncError != "" {
		query.Set("error", state.LastSyncError)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, state.PrimaryURL+"/api/v1/cluster/peer/snapshot?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "VeloraNode "+state.NodeID+":"+state.Secret)
	response, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the primary: %s", friendlyNetError(err))
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusUnauthorized {
		return errors.New("the primary no longer accepts this node; it was removed or the cluster was dissolved — leave the cluster to edit zones locally")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the primary answered HTTP %d%s", response.StatusCode, hostHint(response))
	}
	if !hmacEqual(response.Header.Get("X-Velora-Signature"), sign(signatureKey(state.Secret), body)) {
		return errors.New("the snapshot signature does not match; refusing to apply it")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return errors.New("the primary sent an unreadable snapshot")
	}
	if snapshot.Protocol != Protocol {
		return fmt.Errorf("the primary now speaks cluster protocol %d, this node speaks %d; update this node", snapshot.Protocol, Protocol)
	}
	if snapshot.ClusterID != state.ClusterID {
		return errors.New("the primary belongs to a different cluster")
	}
	if snapshot.Revision == state.AppliedRevision {
		return nil
	}
	if err := s.apply(ctx, snapshot); err != nil {
		return fmt.Errorf("applying revision %s failed: %w", snapshot.Revision, err)
	}
	current := s.current()
	current.AppliedRevision = snapshot.Revision
	return s.save(ctx, current)
}

// apply makes the local authoritative zones equal to the snapshot. Each zone
// is replaced atomically; a failure leaves the previous revision marked as
// applied so the next sync retries.
func (s *Service) apply(ctx context.Context, snapshot Snapshot) error {
	local := map[string]zones.Zone{}
	for _, zone := range s.zones.List() {
		if zone.ZoneType != "secondary" {
			local[zone.Name] = zone
		}
	}
	wanted := map[string]bool{}
	for _, item := range snapshot.Zones {
		wanted[item.Name] = true
		zone := zones.Zone{Name: item.Name, PrimaryNS: item.PrimaryNS, Contact: item.Contact, Records: make([]zones.Record, 0, len(item.Records))}
		for _, record := range item.Records {
			zone.Records = append(zone.Records, zones.Record{Name: record.Name, Type: record.Type, TTL: record.TTL, Value: record.Value, Priority: record.Priority})
		}
		existing, found := local[item.Name]
		if !found {
			if _, err := s.zones.Create(ctx, zone); err != nil {
				return fmt.Errorf("zone %s: %w", item.Name, err)
			}
			continue
		}
		if sameZone(existing, item) {
			continue
		}
		if _, err := s.zones.Update(ctx, existing.ID, existing.Revision, zone); err != nil {
			return fmt.Errorf("zone %s: %w", item.Name, err)
		}
	}
	for name, zone := range local {
		if !wanted[name] {
			if err := s.zones.Delete(ctx, zone.ID, zone.Revision); err != nil {
				return fmt.Errorf("zone %s: %w", name, err)
			}
		}
	}
	return nil
}

func sameZone(local zones.Zone, wanted SnapshotZone) bool {
	current := BuildSnapshot([]zones.Zone{local}, "", SnapshotIdentity{}, time.Time{})
	target := BuildSnapshot([]zones.Zone{{Name: wanted.Name, PrimaryNS: wanted.PrimaryNS, Contact: wanted.Contact, Records: toRecords(wanted.Records)}}, "", SnapshotIdentity{}, time.Time{})
	return current.Revision == target.Revision
}

func toRecords(records []SnapshotRecord) []zones.Record {
	out := make([]zones.Record, 0, len(records))
	for _, record := range records {
		out = append(out, zones.Record{Name: record.Name, Type: record.Type, TTL: record.TTL, Value: record.Value, Priority: record.Priority})
	}
	return out
}

// Info is the public identity served at /api/v1/cluster/peer/info.
func (s *Service) Info() map[string]any {
	state := s.current()
	info := map[string]any{"role": state.Role, "protocol": Protocol, "version": s.version}
	if state.Role == RolePrimary {
		info["cluster_id"] = state.ClusterID
		info["name"] = state.NodeName
	}
	return info
}
