package dns

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	wire "github.com/miekg/dns"
)

type certificateReloader struct {
	certFile, keyFile string
	mu                sync.Mutex
	certificate       *tls.Certificate
	certMod, keyMod   time.Time
}

func TLSConfig(certFile, keyFile string) (*tls.Config, error) {
	r := &certificateReloader{certFile: certFile, keyFile: keyFile}
	if _, err := r.load(); err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return r.load() }}, nil
}

func (r *certificateReloader) load() (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	certInfo, err := os.Stat(r.certFile)
	if err != nil {
		return nil, fmt.Errorf("stat TLS certificate: %w", err)
	}
	keyInfo, err := os.Stat(r.keyFile)
	if err != nil {
		return nil, fmt.Errorf("stat TLS key: %w", err)
	}
	if r.certificate != nil && certInfo.ModTime().Equal(r.certMod) && keyInfo.ModTime().Equal(r.keyMod) {
		return r.certificate, nil
	}
	certificate, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	r.certificate, r.certMod, r.keyMod = &certificate, certInfo.ModTime(), keyInfo.ModTime()
	return r.certificate, nil
}

func StartTLS(address string, handler wire.Handler, config *tls.Config, options ServerOptions) (*Server, error) {
	tcp, err := net.Listen(network(address), address)
	if err != nil {
		return nil, fmt.Errorf("bind DNS-over-TLS: %w", err)
	}
	listener := net.Listener(tls.NewListener(tcp, config))
	if options.MaxTCPConnections > 0 {
		listener = newLimitedListener(listener, options.MaxTCPConnections, options.Observer)
	}
	s := &Server{errors: make(chan error, 1)}
	server := &wire.Server{Listener: listener, Handler: handler, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: func() time.Duration { return 15 * time.Second }, MaxTCPQueries: 100}
	s.servers = append(s.servers, server)
	started := make(chan struct{}, 1)
	server.NotifyStartedFunc = func() { started <- struct{}{} }
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if serveErr := server.ActivateAndServe(); serveErr != nil {
			s.ready.Store(false)
			s.errors <- serveErr
		}
	}()
	select {
	case <-started:
		s.ready.Store(true)
		return s, nil
	case serveErr := <-s.errors:
		_ = listener.Close()
		return nil, serveErr
	}
}

func network(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil && net.ParseIP(host).To4() != nil {
		return "tcp4"
	}
	return "tcp6"
}
