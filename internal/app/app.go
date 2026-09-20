package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/backup"
	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
	"github.com/matta813/velora-dns/internal/update"
	"github.com/matta813/velora-dns/internal/zones"
)

type transferClientAdapter struct{ client *dns.TransferClient }

func (a transferClientAdapter) AXFR(ctx context.Context, zone, primaryAddr, tsigKeyName string) (*zones.TransferResult, error) {
	result, err := a.client.AXFR(ctx, zone, dns.ParseTransferAddress(primaryAddr), tsigKeyName)
	if result == nil {
		return nil, err
	}
	return &zones.TransferResult{Records: result.Records, SOA: result.SOA, Errors: result.Errors}, err
}

func (a transferClientAdapter) IXFR(ctx context.Context, zone, primaryAddr string, serial uint32, tsigKeyName string) (*zones.TransferResult, error) {
	result, err := a.client.IXFR(ctx, zone, dns.ParseTransferAddress(primaryAddr), serial, tsigKeyName)
	if result == nil {
		return nil, err
	}
	return &zones.TransferResult{Records: result.Records, SOA: result.SOA, Errors: result.Errors}, err
}

func Run(ctx context.Context, c config.Config, configPath string, logger *slog.Logger, version api.Version) (result error) {
	if err := c.Validate(); err != nil {
		return err
	}
	tsigStore := dns.NewTSIGStore()
	for _, key := range c.TSIG.Keys {
		if err := tsigStore.AddKey(key.Name, key.Algorithm, key.Secret); err != nil {
			return fmt.Errorf("load TSIG key %q: %w", key.Name, err)
		}
	}
	started := time.Now()
	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	defer initCancel()
	dbPath := c.DatabasePath
	if c.DatabaseDriver == "postgres" {
		dbPath = c.DatabaseURL
	}
	db, err := database.Open(initCtx, c.DatabaseDriver, dbPath)
	if err != nil {
		return fmt.Errorf("open management database: %w", err)
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	userCount, err := db.UserCount(initCtx)
	if err != nil {
		return fmt.Errorf("inspect management users: %w", err)
	}
	if userCount == 0 {
		if c.Management.BootstrapUsername == "" {
			logger.Warn("management locked: no users exist; restart with bootstrap credentials to enable access")
		} else {
			if _, err = db.CreateUser(initCtx, c.Management.BootstrapUsername, c.Management.BootstrapPassword, "admin"); err != nil {
				return fmt.Errorf("bootstrap management admin: %w", err)
			}
			logger.Info("management admin bootstrapped", "username", c.Management.BootstrapUsername)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	memory := cache.New(c.Cache.MaxEntries)
	local, err := zones.New(initCtx, db, memory.Flush)
	if err != nil {
		return fmt.Errorf("load local zones: %w", err)
	}
	secondary := zones.NewSecondaryManager(local, transferClientAdapter{client: dns.NewTransferClient(tsigStore, 30*time.Second)}, logger)
	secondary.Start(runCtx)
	defer func() {
		cancel()
		secondary.Stop()
	}()
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
	audit := querylog.New(db, c.QueryLog.Enabled, c.QueryLog.QueueSize, c.QueryLog.Retention, c.QueryLog.MaxRows)
	observer.ObserveQueryLog(audit)
	auditCtx, auditCancel := context.WithCancel(context.Background())
	var auditWG sync.WaitGroup
	auditWG.Go(func() { audit.Run(auditCtx) })
	defer func() { auditCancel(); auditWG.Wait() }()

	updateManager := update.NewManager(update.DefaultConfig())
	if err := updateManager.LoadState(); err != nil {
		return fmt.Errorf("load update state: %w", err)
	}
	defer func() { _ = updateManager.SaveState() }()

	backupManager := backup.NewManager(db, c.DatabasePath)

	var wg sync.WaitGroup
	wg.Go(func() { memory.Run(runCtx) })
	defer func() { cancel(); wg.Wait() }()
	var allowed []netip.Prefix
	for _, cidr := range c.DNS.AllowedClients {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return err
		}
		allowed = append(allowed, p)
	}
	var validator *dns.DNSSECValidator
	if c.DNS.DNSSEC {
		validator, err = dns.NewDNSSECValidator(c.DNS.TrustAnchors)
		if err != nil {
			return fmt.Errorf("initialize DNSSEC: %w", err)
		}
	}
	forwarder := &dns.Forwarder{Upstreams: c.DNS.Upstreams, Timeout: c.DNS.Timeout, Retries: c.DNS.Retries, Observer: observer, Validator: validator}
	resolver := dns.NewResolver(local, matcher, memory, forwarder, c.Filtering.BlockMode)
	cookieSecret := make([]byte, 32)
	if _, err = rand.Read(cookieSecret); err != nil {
		return fmt.Errorf("initialize DNS cookie secret: %w", err)
	}
	rateLimitState := dns.NewRateLimitState(c.DNS.RateLimitEnabled, c.DNS.GlobalQPS, c.DNS.ClientQPS, c.DNS.RateLimitBurst)
	if persisted, err := db.GetRateLimitSettings(initCtx); err == nil {
		if persisted.Enabled || persisted.GlobalQPS > 0 || persisted.ClientQPS > 0 || persisted.RateLimitBurst > 0 {
			rateLimitState.Configure(persisted.Enabled, persisted.GlobalQPS, persisted.ClientQPS, persisted.RateLimitBurst)
		}
	}
	dnsHandler := &dns.Handler{Context: runCtx, Resolver: resolver, Allowed: allowed, Slots: make(chan struct{}, c.DNS.MaxConcurrent), RateLimit: rateLimitState, Observer: observer, Audit: audit, CookieSecret: cookieSecret}
	applyConfig := func(updated config.Config) error {
		audit.SetEnabled(updated.QueryLog.Enabled)
		rateLimitState.Configure(updated.DNS.RateLimitEnabled, updated.DNS.GlobalQPS, updated.DNS.ClientQPS, updated.DNS.RateLimitBurst)
		newAllowed := make([]netip.Prefix, 0, len(updated.DNS.AllowedClients))
		for _, cidr := range updated.DNS.AllowedClients {
			prefix, err := netip.ParsePrefix(cidr)
			if err != nil {
				return err
			}
			newAllowed = append(newAllowed, prefix)
		}
		dnsHandler.UpdateConfig(newAllowed, updated.DNS.MaxConcurrent)
		resolver.SetBlockMode(updated.Filtering.BlockMode)
		return nil
	}
	listener, err := dns.StartWithOptions(c.DNS.Listen, dnsHandler, dns.ServerOptions{MaxTCPConnections: c.DNS.MaxTCPConns, Observer: observer})
	if err != nil {
		return err
	}
	defer func() {
		cancel()
		shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		result = errors.Join(result, listener.Shutdown(shutdown))
	}()
	var dotListener *dns.Server
	var dotErrors <-chan error
	var dohServer *http.Server
	var dohErrors <-chan error
	var doqServer *dns.QUICServer
	var doqErrors <-chan error
	if c.DNS.DoTListen != "" || c.DNS.DoHListen != "" {
		tlsConfig, tlsErr := dns.TLSConfig(c.DNS.TLSCertFile, c.DNS.TLSKeyFile)
		if tlsErr != nil {
			return tlsErr
		}
		if c.DNS.DoTListen != "" {
			dotListener, err = dns.StartTLS(c.DNS.DoTListen, dnsHandler, tlsConfig.Clone(), dns.ServerOptions{MaxTCPConnections: c.DNS.MaxTCPConns, Observer: observer})
			if err != nil {
				return err
			}
			dotErrors = dotListener.Errors()
			defer func() {
				shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				result = errors.Join(result, dotListener.Shutdown(shutdown))
			}()
		}
		if c.DNS.DoHListen != "" {
			dohSocket, listenErr := net.Listen("tcp", c.DNS.DoHListen)
			if listenErr != nil {
				return fmt.Errorf("bind DNS-over-HTTPS: %w", listenErr)
			}
			dohServer = &http.Server{Handler: dns.DoH(dnsHandler), TLSConfig: tlsConfig.Clone(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
			errorsChannel := make(chan error, 1)
			dohErrors = errorsChannel
			go func() { errorsChannel <- dohServer.ServeTLS(dohSocket, "", "") }()
			defer func() {
				shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				result = errors.Join(result, dohServer.Shutdown(shutdown))
			}()
		}
	}
	if c.DNS.DoQListen != "" {
		tlsConfig, tlsErr := dns.TLSConfig(c.DNS.TLSCertFile, c.DNS.TLSKeyFile)
		if tlsErr != nil {
			return tlsErr
		}
		doqServer, err = dns.StartQUIC(c.DNS.DoQListen, dnsHandler, tlsConfig)
		if err != nil {
			return err
		}
		doqErrors = doqServer.Errors()
		defer func() {
			shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			result = errors.Join(result, doqServer.Shutdown(shutdown))
		}()
	}
	httpNetwork := "tcp6"
	if address, parseErr := netip.ParseAddrPort(c.HTTP.Listen); parseErr == nil && address.Addr().Is4() {
		httpNetwork = "tcp4"
	}
	socket, err := net.Listen(httpNetwork, c.HTTP.Listen)
	if err != nil {
		return fmt.Errorf("bind management HTTP: %w", err)
	}
	server := &http.Server{Handler: api.New(api.Dependencies{Database: db, Auth: db, Zones: local, Filtering: matcher, Queries: db, DNS: listener, Cache: memory, Metrics: observer, Config: c, ConfigPath: configPath, Version: version, Started: started, TSIG: tsigStore, Settings: db, RateLimit: rateLimitState, Update: updateManager, Backup: backupManager, Onboarding: db, ApplyConfig: applyConfig}), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	httpErrors := make(chan error, 1)
	go func() { httpErrors <- server.Serve(socket) }()
	logger.Info("server started", "dns_listen", listener.Addresses(), "http_listen", socket.Addr().String(), "version", version.Version)
	select {
	case <-ctx.Done():
	case err = <-listener.Errors():
		result = fmt.Errorf("DNS listener: %w", err)
	case err = <-dotErrors:
		result = fmt.Errorf("DNS-over-TLS listener: %w", err)
	case err = <-dohErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			result = fmt.Errorf("DNS-over-HTTPS listener: %w", err)
		}
	case err = <-doqErrors:
		result = fmt.Errorf("DNS-over-QUIC listener: %w", err)
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
