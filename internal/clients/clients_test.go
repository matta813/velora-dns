package clients

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
)

type memoryStore struct {
	clients []Client
	next    int64
}

func (m *memoryStore) LoadClients(context.Context) ([]Client, error) {
	return slices.Clone(m.clients), nil
}
func (m *memoryStore) SaveClient(_ context.Context, client Client) (Client, error) {
	if client.ID == 0 {
		m.next++
		client.ID = m.next
		m.clients = append(m.clients, client)
		return client, nil
	}
	for i := range m.clients {
		if m.clients[i].ID == client.ID {
			m.clients[i] = client
			return client, nil
		}
	}
	return client, ErrNotFound
}
func (m *memoryStore) DeleteClient(_ context.Context, id int64) error {
	for i := range m.clients {
		if m.clients[i].ID == id {
			m.clients = slices.Delete(m.clients, i, i+1)
			return nil
		}
	}
	return ErrNotFound
}

func newService(t *testing.T, clients ...Client) *Service {
	t.Helper()
	s, err := NewService(context.Background(), &memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range clients {
		client.Enabled = true
		if _, err := s.Create(context.Background(), client); err != nil {
			t.Fatalf("create %s: %v", client.Name, err)
		}
	}
	return s
}

func TestParsePrefix(t *testing.T) {
	for raw, want := range map[string]string{
		"192.168.1.20":        "192.168.1.20/32",
		"192.168.1.77/24":     "192.168.1.0/24",
		"::ffff:192.168.1.5":  "192.168.1.5/32",
		"::ffff:10.0.0.0/104": "10.0.0.0/8",
		"2001:db8::1":         "2001:db8::1/128",
		" 2001:db8:1::/48 ":   "2001:db8:1::/48",
	} {
		got, err := ParsePrefix(raw)
		if err != nil || got.String() != want {
			t.Errorf("%q: got %v %v, want %s", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "tv.local", "10.0.0.0/33", "fe80::1%eth0"} {
		if _, err := ParsePrefix(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: expected ErrInvalid, got %v", raw, err)
		}
	}
}

func TestLongestPrefixWins(t *testing.T) {
	s := newService(t,
		Client{Name: "Home network", Addresses: []string{"192.168.1.0/24", "2001:db8:1::/48"}, Group: "Networks"},
		Client{Name: "Kids", Addresses: []string{"192.168.1.128/25"}},
		Client{Name: "Living Room TV", Addresses: []string{"192.168.1.200", "2001:db8:1::200"}, Group: "Media"},
	)
	for ip, want := range map[string]string{
		"192.168.1.10":         "Home network",
		"192.168.1.150":        "Kids",
		"192.168.1.200":        "Living Room TV",
		"::ffff:192.168.1.200": "Living Room TV",
		"2001:db8:1::200":      "Living Room TV",
		"2001:db8:1::5":        "Home network",
		"192.168.2.1":          "",
		"2001:db8:2::1":        "",
	} {
		if got := s.Name(ip); got != want {
			t.Errorf("%s: got %q, want %q", ip, got, want)
		}
	}
	if s.Name("not an ip") != "" {
		t.Error("invalid address named")
	}
}

func TestValidationUniquenessAndDisable(t *testing.T) {
	s := newService(t, Client{Name: "NAS", Addresses: []string{"192.168.1.20"}})
	ctx := context.Background()
	for label, client := range map[string]Client{
		"no name":      {Addresses: []string{"192.168.1.21"}},
		"no addresses": {Name: "Printer"},
		"bad address":  {Name: "Printer", Addresses: []string{"printer.local"}},
		"multiline":    {Name: "Printer", Addresses: []string{"192.168.1.21"}, Description: "a\nb"},
	} {
		if _, err := s.Create(ctx, client); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: expected ErrInvalid, got %v", label, err)
		}
	}
	if _, err := s.Create(ctx, Client{Name: "nas", Addresses: []string{"192.168.1.30"}}); !errors.Is(err, ErrExists) {
		t.Fatalf("names are unique regardless of case: %v", err)
	}
	if _, err := s.Create(ctx, Client{Name: "Other", Addresses: []string{"192.168.1.20/32"}}); !errors.Is(err, ErrExists) {
		t.Fatalf("a network may belong to only one client: %v", err)
	}
	nas := s.clients[0]
	updated, err := s.Update(ctx, nas.ID, Client{Name: "NAS", Addresses: []string{"192.168.1.20", "192.168.1.20", "2001:db8::20"}, Enabled: false})
	if err != nil || len(updated.Addresses) != 2 {
		t.Fatalf("update with duplicate addresses: %+v %v", updated, err)
	}
	if _, ok := s.Lookup(netip.MustParseAddr("192.168.1.20")); ok {
		t.Fatal("disabled clients must not match")
	}
	if err = s.Delete(ctx, nas.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, nas.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
