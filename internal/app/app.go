package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
	"github.com/matta813/velora-dns/internal/zones"
)

func Run(ctx context.Context, c config.Config, logger *slog.Logger, version api.Version) (result error) {
	if err := c.Validate(); err != nil {
		return err
	}
	started := time.Now()
	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	defer initCancel()
	db, err := database.Open(initCtx, c.DatabasePath)
	if err != nil {
		return fmt.Errorf("open management database: %w", err)
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	memory := cache.New(c.Cache.MaxEntries)
	local, err := zones.New(initCtx, db, memory.Flush)
	if err != nil {
		return fmt.Errorf("load local zones: %w", err)
	}
	rules := make([]filtering.Rule, 0, len(c.Filtering.Blocklist)+len(c.Filtering.Allowlist))
	for _, domain := range c.Filtering.Blocklist {
		rules = append(rules, filtering.Rule{Domain: domain, Wildcard: true, Action: filtering.Block})
	}
	for _, domain := range c.Filtering.Allowlist {
		rules = append(rules, filtering.Rule{Domain: domain, Action: filtering.Allow})
	}
	matcher, err := filtering.NewService(initCtx, db, rules)
	if err != nil {
		return fmt.Errorf("load filtering rules: %w", err)
	}
	observer := metrics.New(memory)
	audit := querylog.New(db, c.QueryLog.Enabled, c.QueryLog.QueueSize, c.QueryLog.Retention)
	var wg sync.WaitGroup
	wg.Go(func() { memory.Run(runCtx) })
	wg.Go(func() { audit.Run(runCtx) })
	defer func() { cancel(); wg.Wait() }()
	var allowed []netip.Prefix
	for _, cidr := range c.DNS.AllowedClients {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return err
		}
		allowed = append(allowed, p)
	}
	forwarder := &dns.Forwarder{Upstreams: c.DNS.Upstreams, Timeout: c.DNS.Timeout, Retries: c.DNS.Retries, Observer: observer}
	resolver := &dns.Resolver{Cache: memory, Forwarder: forwarder, Local: local, Filter: matcher}
	listener, err := dns.Start(c.DNS.Listen, &dns.Handler{Context: runCtx, Resolver: resolver, Allowed: allowed, Slots: make(chan struct{}, c.DNS.MaxConcurrent), Observer: observer, Audit: audit})
	if err != nil {
		return err
	}
	defer func() {
		cancel()
		shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		result = errors.Join(result, listener.Shutdown(shutdown))
	}()
	httpNetwork := "tcp6"
	if address, parseErr := netip.ParseAddrPort(c.HTTP.Listen); parseErr == nil && address.Addr().Is4() {
		httpNetwork = "tcp4"
	}
	socket, err := net.Listen(httpNetwork, c.HTTP.Listen)
	if err != nil {
		return fmt.Errorf("bind management HTTP: %w", err)
	}
	server := &http.Server{Handler: api.New(api.Dependencies{Database: db, Zones: local, Filtering: matcher, Queries: db, DNS: listener, Cache: memory, Metrics: observer, Config: c, Version: version, Started: started}), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	httpErrors := make(chan error, 1)
	go func() { httpErrors <- server.Serve(socket) }()
	logger.Info("server started", "dns_listen", listener.Addresses(), "http_listen", socket.Addr().String(), "version", version.Version)
	select {
	case <-ctx.Done():
	case err = <-listener.Errors():
		result = fmt.Errorf("DNS listener: %w", err)
	case err = <-httpErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			result = fmt.Errorf("HTTP listener: %w", err)
		}
	}
	logger.Info("server stopping")
	cancel()
	shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		result = errors.Join(result, err)
	}
	return result
}
