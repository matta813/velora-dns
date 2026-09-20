package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	wire "github.com/miekg/dns"
)

type Server struct {
	servers []*wire.Server
	ready   atomic.Bool
	wg      sync.WaitGroup
	errors  chan error
}
type ServerOptions struct {
	MaxTCPConnections int
	Observer          OverloadObserver
}

func (s *Server) Ready() bool          { return s.ready.Load() }
func (s *Server) Errors() <-chan error { return s.errors }

// Start binds every UDP/TCP socket before reporting readiness. Port zero is supported for tests.
func Start(addresses []string, handler wire.Handler) (*Server, error) {
	return StartWithOptions(addresses, handler, ServerOptions{})
}

func StartWithOptions(addresses []string, handler wire.Handler, options ServerOptions) (*Server, error) {
	s := &Server{errors: make(chan error, len(addresses)*2)}
	cleanup := func() {
		for _, server := range s.servers {
			if server.Listener != nil {
				_ = server.Listener.Close()
			}
			if server.PacketConn != nil {
				_ = server.PacketConn.Close()
			}
		}
	}
	for _, address := range addresses {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("invalid listener: %w", err)
		}
		tcpNetwork, udpNetwork := "tcp", "udp"
		if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
			tcpNetwork, udpNetwork = "tcp4", "udp4"
		} else if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
			tcpNetwork, udpNetwork = "tcp6", "udp6"
		}
		tcp, err := net.Listen(tcpNetwork, address)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("bind TCP: %w", err)
		}
		udp, err := net.ListenPacket(udpNetwork, tcp.Addr().String())
		if err != nil {
			_ = tcp.Close()
			cleanup()
			return nil, fmt.Errorf("bind UDP: %w", err)
		}
		listener := net.Listener(tcp)
		if options.MaxTCPConnections > 0 {
			listener = newLimitedListener(tcp, options.MaxTCPConnections, options.Observer)
		}
		s.servers = append(s.servers, &wire.Server{Listener: listener, Handler: handler, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: func() time.Duration { return 5 * time.Second }, MaxTCPQueries: 100}, &wire.Server{PacketConn: udp, Handler: handler, UDPSize: 1232})
	}
	started := make(chan struct{}, len(s.servers))
	for _, server := range s.servers {
		server.NotifyStartedFunc = func() { started <- struct{}{} }
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := server.ActivateAndServe(); err != nil {
				s.ready.Store(false)
				s.errors <- err
			}
		}()
	}
	for range s.servers {
		select {
		case <-started:
		case err := <-s.errors:
			cleanup()
			return nil, err
		}
	}
	s.ready.Store(true)
	return s, nil
}
func (s *Server) Addresses() []string {
	var out []string
	for _, server := range s.servers {
		if server.Listener != nil {
			out = append(out, server.Listener.Addr().String())
		}
	}
	return out
}
func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	var errs []error
	for _, server := range s.servers {
		if err := server.ShutdownContext(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	s.wg.Wait()
	return errors.Join(errs...)
}
