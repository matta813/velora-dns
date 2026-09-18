package zones

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

type secondaryRepo struct {
	mu   sync.Mutex
	zone Zone
}

func (r *secondaryRepo) LoadZones(context.Context) ([]Zone, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []Zone{clone(r.zone)}, nil
}

func (r *secondaryRepo) SaveZone(_ context.Context, zone Zone, _ uint32) (Zone, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range zone.Records {
		zone.Records[i].ID = int64(i + 1)
	}
	r.zone = clone(zone)
	return clone(zone), nil
}

func (r *secondaryRepo) DeleteZone(context.Context, int64, uint32) error { return nil }

type successfulTransfer struct{}

func (successfulTransfer) AXFR(context.Context, string, string, string) (*TransferResult, error) {
	return &TransferResult{
		SOA: &wire.SOA{Hdr: wire.RR_Header{Name: "secondary.test.", Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 60}, Ns: "ns.primary.test.", Mbox: "hostmaster.primary.test.", Serial: 42},
		Records: []wire.RR{
			&wire.SOA{Hdr: wire.RR_Header{Name: "secondary.test.", Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 60}},
			&wire.A{Hdr: wire.RR_Header{Name: "host.secondary.test.", Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 120}, A: []byte{192, 0, 2, 44}},
		},
	}, nil
}

func (successfulTransfer) IXFR(context.Context, string, string, uint32, string) (*TransferResult, error) {
	return nil, nil
}

func TestSecondaryManagerAppliesInitialTransfer(t *testing.T) {
	repo := &secondaryRepo{zone: Zone{ID: 1, Name: "secondary.test.", PrimaryNS: "ns.secondary.test.", Contact: "hostmaster.secondary.test.", Revision: 1, ZoneType: "secondary", PrimaryAddress: "192.0.2.53:53", TransferInterval: 3600}}
	service, err := New(context.Background(), repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewSecondaryManager(service, successfulTransfer{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	t.Cleanup(func() { cancel(); manager.Stop() })

	deadline := time.Now().Add(2 * time.Second)
	for {
		zone, getErr := service.Get(1)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if zone.LastTransferSerial == 42 && len(zone.Records) == 1 {
			if zone.Records[0].Value != "192.0.2.44" || zone.NextRefreshAt == nil || zone.Revision != 2 {
				t.Fatalf("unexpected transferred zone: %+v", zone)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("transfer was not applied: %+v", zone)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
