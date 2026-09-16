package zones

import (
	"context"
	"log/slog"
	"sync"
	"time"

	wire "github.com/miekg/dns"
)

// TransferClient performs zone transfers from primary servers.
type TransferClient interface {
	AXFR(ctx context.Context, zone, primaryAddr, tsigKeyName string) (*TransferResult, error)
	IXFR(ctx context.Context, zone, primaryAddr string, serial uint32, tsigKeyName string) (*TransferResult, error)
}

// TransferResult contains the result of a zone transfer.
type TransferResult struct {
	Records []wire.RR
	SOA     *wire.SOA
	Errors  []error
}

// SecondaryManager manages secondary zone transfers and SOA lifecycle.
type SecondaryManager struct {
	repo      Repository
	service   *Service
	client    TransferClient
	logger    *slog.Logger
	mu        sync.Mutex
	transfers map[int64]context.CancelFunc
}

// NewSecondaryManager creates a new secondary zone manager.
func NewSecondaryManager(repo Repository, service *Service, client TransferClient, logger *slog.Logger) *SecondaryManager {
	return &SecondaryManager{
		repo:      repo,
		service:   service,
		client:    client,
		logger:    logger,
		transfers: make(map[int64]context.CancelFunc),
	}
}

// Start begins monitoring secondary zones for transfers.
func (m *SecondaryManager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	go m.monitorLoop(ctx)
}

// Stop stops all transfer goroutines.
func (m *SecondaryManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cancel := range m.transfers {
		cancel()
	}
}

func (m *SecondaryManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkTransfers(ctx)
		}
	}
}

func (m *SecondaryManager) checkTransfers(ctx context.Context) {
	zones := m.service.List()
	for _, z := range zones {
		if z.ZoneType != "secondary" || z.PrimaryAddress == "" {
			continue
		}
		if z.NextRefreshAt != nil && z.NextRefreshAt.After(time.Now()) {
			continue
		}
		m.startTransfer(ctx, z)
	}
}

func (m *SecondaryManager) startTransfer(ctx context.Context, z Zone) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.transfers[z.ID]; exists {
		return
	}

	transferCtx, cancel := context.WithCancel(ctx)
	m.transfers[z.ID] = cancel

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.transfers, z.ID)
			m.mu.Unlock()
		}()
		m.performTransfer(transferCtx, z)
	}()
}

func (m *SecondaryManager) performTransfer(ctx context.Context, z Zone) {
	m.logger.Info("starting zone transfer", "zone", z.Name, "primary", z.PrimaryAddress)

	var result *TransferResult
	var err error

	if z.LastTransferSerial > 0 {
		result, err = m.client.IXFR(ctx, z.Name, z.PrimaryAddress, z.LastTransferSerial, z.TransferTSIGKey)
	} else {
		result, err = m.client.AXFR(ctx, z.Name, z.PrimaryAddress, z.TransferTSIGKey)
	}

	if err != nil {
		m.logger.Error("transfer failed", "zone", z.Name, "error", err)
		m.updateTransferState(z, false, nil)
		return
	}

	if len(result.Errors) > 0 {
		m.logger.Error("transfer errors", "zone", z.Name, "errors", result.Errors)
		m.updateTransferState(z, false, nil)
		return
	}

	if result.SOA != nil {
		m.updateTransferState(z, true, result.SOA)
	} else {
		m.updateTransferState(z, true, nil)
	}

	m.logger.Info("zone transfer complete", "zone", z.Name, "records", len(result.Records))
}

func (m *SecondaryManager) updateTransferState(z Zone, success bool, soa *wire.SOA) {
	now := time.Now()
	z.LastTransferAt = &now

	if soa != nil {
		z.LastTransferSerial = soa.Serial
	}

	interval := z.TransferInterval
	if interval <= 0 {
		interval = 3600
	}

	if success {
		refresh := interval
		nextRefresh := now.Add(time.Duration(refresh) * time.Second)
		z.NextRefreshAt = &nextRefresh
	} else {
		retry := interval / 6
		if retry < 60 {
			retry = 60
		}
		nextRefresh := now.Add(time.Duration(retry) * time.Second)
		z.NextRefreshAt = &nextRefresh
	}

	if _, err := m.repo.SaveZone(context.Background(), z, z.Revision); err != nil {
		m.logger.Error("failed to update transfer state", "zone", z.Name, "error", err)
	}
}
