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
type TransportObserver interface {
	Overload(string)
	TCPConnection(bool)
}

func (s *Server) Ready() bool          { return s.ready.Load() }
func (s *Server) Errors() <-chan error { return s.errors }

// Start binds every UDP/TCP socket before reporting readiness. Port zero is supported for tests.
func Start(addresses []string, handler wire.Handler) (*Server, error) {
	return StartWithLimits(addresses, handler, 256, nil)
}

func StartWithLimits(addresses []string, handler wire.Handler, maxTCPConnections int, observer TransportObserver) (*Server, error) {
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
		tcpNetwork, udpNetwork := "tcp6", "udp6"
		if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
			tcpNetwork, udpNetwork = "tcp4", "udp4"
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
		limited := &limitedListener{Listener: tcp, slots: make(chan struct{}, maxTCPConnections), observer: observer}
		s.servers = append(s.servers, &wire.Server{Listener: limited, Handler: handler, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: func() time.Duration { return 5 * time.Second }, MaxTCPQueries: 100}, &wire.Server{PacketConn: udp, Handler: handler, UDPSize: 1232})
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

type limitedListener struct {
	net.Listener
	slots    chan struct{}
	observer TransportObserver
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			if l.observer != nil {
				l.observer.TCPConnection(true)
			}
			return &limitedConn{Conn: conn, release: func() {
				<-l.slots
				if l.observer != nil {
					l.observer.TCPConnection(false)
				}
			}}, nil
		default:
			_ = conn.Close()
			if l.observer != nil {
				l.observer.Overload("tcp_connections")
			}
		}
	}
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
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
