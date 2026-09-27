// Package clients gives client IP addresses and networks friendly names that
// the query log, statistics and per-client policies can share.
package clients

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	MaxClients        = 1024
	MaxAddresses      = 16
	maxNameLength     = 80
	maxGroupLength    = 60
	maxDescriptionLen = 200
)

var (
	ErrInvalid  = errors.New("invalid client")
	ErrNotFound = errors.New("client not found")
	ErrExists   = errors.New("client already exists")
)

// Client names one or more addresses or networks. Addresses are stored as
// prefixes; single hosts are shown without a prefix length.
type Client struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Addresses   []string `json:"addresses"`
	Group       string   `json:"group"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
}

type Store interface {
	LoadClients(context.Context) ([]Client, error)
	SaveClient(context.Context, Client) (Client, error)
	DeleteClient(context.Context, int64) error
}

type index struct {
	// byLength maps a prefix length to the clients owning prefixes of that
	// length; lengths is sorted longest first for longest-prefix matching.
	byLength map[int]map[netip.Prefix]Client
	lengths  []int
}

type Service struct {
	store   Store
	mu      sync.Mutex
	clients []Client
	current atomic.Pointer[index]
}

func NewService(ctx context.Context, store Store) (*Service, error) {
	clients, err := store.LoadClients(ctx)
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, clients: clients}
	s.current.Store(build(clients))
	return s, nil
}

func build(clients []Client) *index {
	idx := &index{byLength: map[int]map[netip.Prefix]Client{}}
	for _, client := range clients {
		if !client.Enabled {
			continue
		}
		for _, raw := range client.Addresses {
			prefix, err := ParsePrefix(raw)
			if err != nil {
				continue
			}
			bucket := idx.byLength[prefix.Bits()]
			if bucket == nil {
				bucket = map[netip.Prefix]Client{}
				idx.byLength[prefix.Bits()] = bucket
				idx.lengths = append(idx.lengths, prefix.Bits())
			}
			bucket[prefix] = client
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idx.lengths)))
	return idx
}

// Lookup returns the enabled client whose most specific network contains ip.
func (s *Service) Lookup(ip netip.Addr) (Client, bool) {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return Client{}, false
	}
	idx := s.current.Load()
	for _, bits := range idx.lengths {
		if bits > ip.BitLen() {
			continue
		}
		prefix, err := ip.Prefix(bits)
		if err != nil {
			continue
		}
		if client, ok := idx.byLength[bits][prefix]; ok {
			return client, true
		}
	}
	return Client{}, false
}

// Name returns the friendly name for a textual IP, or "" when unknown.
func (s *Service) Name(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	if client, ok := s.Lookup(addr); ok {
		return client.Name
	}
	return ""
}

func (s *Service) List(context.Context) ([]Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.clients), nil
}

func (s *Service) Get(id int64) (Client, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.clients {
		if client.ID == id {
			return client, true
		}
	}
	return Client{}, false
}

func (s *Service) Create(ctx context.Context, client Client) (Client, error) {
	client.ID = 0
	return s.save(ctx, client)
}

func (s *Service) Update(ctx context.Context, id int64, client Client) (Client, error) {
	if id < 1 {
		return Client{}, ErrNotFound
	}
	client.ID = id
	return s.save(ctx, client)
}

func (s *Service) save(ctx context.Context, client Client) (Client, error) {
	normalized, prefixes, err := normalize(client)
	if err != nil {
		return Client{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, existing := range s.clients {
		if existing.ID == normalized.ID && normalized.ID != 0 {
			index = i
			continue
		}
		if strings.EqualFold(existing.Name, normalized.Name) {
			return Client{}, fmt.Errorf("%w: a client named %q already exists", ErrExists, existing.Name)
		}
		for _, raw := range existing.Addresses {
			other, err := ParsePrefix(raw)
			if err == nil && slices.Contains(prefixes, other) {
				return Client{}, fmt.Errorf("%w: %s already belongs to %s", ErrExists, raw, existing.Name)
			}
		}
	}
	if normalized.ID != 0 && index < 0 {
		return Client{}, ErrNotFound
	}
	if normalized.ID == 0 && len(s.clients) >= MaxClients {
		return Client{}, fmt.Errorf("%w: at most %d clients", ErrInvalid, MaxClients)
	}
	saved, err := s.store.SaveClient(ctx, normalized)
	if err != nil {
		return Client{}, err
	}
	candidate := slices.Clone(s.clients)
	if index < 0 {
		candidate = append(candidate, saved)
	} else {
		candidate[index] = saved
	}
	sort.SliceStable(candidate, func(i, j int) bool { return strings.ToLower(candidate[i].Name) < strings.ToLower(candidate[j].Name) })
	s.clients = candidate
	s.current.Store(build(candidate))
	return saved, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.clients, func(client Client) bool { return client.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if err := s.store.DeleteClient(ctx, id); err != nil {
		return err
	}
	s.clients = slices.Delete(slices.Clone(s.clients), i, i+1)
	s.current.Store(build(s.clients))
	return nil
}

// ParsePrefix accepts an IP address or CIDR network and returns the masked
// prefix; single addresses become /32 or /128 prefixes.
func ParsePrefix(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.Addr().Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("%w: %q is not a valid network", ErrInvalid, raw)
		}
		addr := prefix.Addr()
		bits := prefix.Bits()
		if addr.Is4In6() && bits >= 96 {
			addr, bits = addr.Unmap(), bits-96
		}
		return netip.PrefixFrom(addr, bits).Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%w: %q is not a valid IP address or network", ErrInvalid, raw)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// FormatPrefix shows host prefixes as plain addresses.
func FormatPrefix(prefix netip.Prefix) string {
	if prefix.IsSingleIP() {
		return prefix.Addr().String()
	}
	return prefix.String()
}

func normalize(client Client) (Client, []netip.Prefix, error) {
	client.Name = strings.TrimSpace(client.Name)
	client.Group = strings.TrimSpace(client.Group)
	client.Description = strings.TrimSpace(client.Description)
	for _, field := range []struct {
		value, label string
		max          int
		required     bool
	}{{client.Name, "name", maxNameLength, true}, {client.Group, "group", maxGroupLength, false}, {client.Description, "description", maxDescriptionLen, false}} {
		if (field.required && field.value == "") || len(field.value) > field.max || strings.ContainsAny(field.value, "\r\n\x00") {
			return Client{}, nil, fmt.Errorf("%w: %s must be a single line of at most %d characters", ErrInvalid, field.label, field.max)
		}
	}
	if len(client.Addresses) == 0 || len(client.Addresses) > MaxAddresses {
		return Client{}, nil, fmt.Errorf("%w: add between 1 and %d addresses or networks", ErrInvalid, MaxAddresses)
	}
	prefixes := make([]netip.Prefix, 0, len(client.Addresses))
	addresses := make([]string, 0, len(client.Addresses))
	for _, raw := range client.Addresses {
		prefix, err := ParsePrefix(raw)
		if err != nil {
			return Client{}, nil, err
		}
		if slices.Contains(prefixes, prefix) {
			continue
		}
		prefixes = append(prefixes, prefix)
		addresses = append(addresses, FormatPrefix(prefix))
	}
	client.Addresses = addresses
	return client, prefixes, nil
}
