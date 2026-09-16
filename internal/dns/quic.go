package dns

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	wire "github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// QUICServer implements DNS-over-QUIC (RFC 9250).
type QUICServer struct {
	listener *quic.Listener
	handler  wire.Handler
	wg       sync.WaitGroup
	errors   chan error
	ready    sync.WaitGroup
}

// StartQUIC starts a DNS-over-QUIC server.
func StartQUIC(address string, handler wire.Handler, config *tls.Config) (*QUICServer, error) {
	if config == nil {
		config = &tls.Config{
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"doq"},
		}
	}
	if len(config.NextProtos) == 0 {
		config.NextProtos = []string{"doq"}
	}

	listener, err := quic.ListenAddr(address, config, &quic.Config{
		MaxIdleTimeout:        30 * time.Second,
		MaxIncomingStreams:    256,
		MaxIncomingUniStreams: 256,
		KeepAlivePeriod:       10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("listen QUIC: %w", err)
	}

	s := &QUICServer{
		listener: listener,
		handler:  handler,
		errors:   make(chan error, 1),
	}

	s.ready.Add(1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.serve()
	}()

	return s, nil
}

func (s *QUICServer) serve() {
	s.ready.Done()
	for {
		conn, err := s.listener.Accept(context.Background())
		if err != nil {
			select {
			case s.errors <- err:
			default:
			}
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

func (s *QUICServer) handleConnection(conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleStream(stream, conn)
		}()
	}
}

func (s *QUICServer) handleStream(stream *quic.Stream, conn *quic.Conn) {
	buf := make([]byte, 65536)
	n, err := stream.Read(buf)
	if err != nil {
		return
	}
	buf = buf[:n]

	msg := new(wire.Msg)
	if err := msg.Unpack(buf); err != nil {
		return
	}

	w := &quicWriter{stream: stream, remoteAddr: conn.RemoteAddr()}
	s.handler.ServeDNS(w, msg)
}

// QUICServer interface methods for the Server adapter.
func (s *QUICServer) Ready() bool          { return s.listener != nil }
func (s *QUICServer) Errors() <-chan error { return s.errors }
func (s *QUICServer) Shutdown(ctx context.Context) error {
	err := s.listener.Close()
	s.wg.Wait()
	return err
}
func (s *QUICServer) Addresses() []string {
	if s.listener == nil {
		return nil
	}
	return []string{s.listener.Addr().String()}
}

// quicWriter implements wire.ResponseWriter for QUIC streams.
type quicWriter struct {
	stream     *quic.Stream
	remoteAddr net.Addr
	localAddr  net.Addr
	tsigOnly   bool
}

func (w *quicWriter) WriteMsg(msg *wire.Msg) error {
	buf, err := msg.Pack()
	if err != nil {
		return err
	}
	_, err = w.stream.Write(buf)
	return err
}

func (w *quicWriter) Write(b []byte) (int, error) {
	return w.stream.Write(b)
}

func (w *quicWriter) Close() error {
	return w.stream.Close()
}

func (w *quicWriter) RemoteAddr() net.Addr {
	return w.remoteAddr
}

func (w *quicWriter) LocalAddr() net.Addr {
	if w.localAddr != nil {
		return w.localAddr
	}
	return &net.UDPAddr{IP: net.IPv4zero, Port: 0}
}

func (w *quicWriter) TsigStatus() error     { return nil }
func (w *quicWriter) TsigTimersOnly(b bool) { w.tsigOnly = b }
func (w *quicWriter) Hijack()               {}
