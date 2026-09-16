package dns

import (
	"net"
	"sync"
)

// limitedListener counts live accepted TCP connections. Connections beyond the
// configured cap are promptly closed, preventing idle TCP peers from consuming
// unbounded goroutines and file descriptors in the DNS server.
type limitedListener struct {
	net.Listener
	sem      chan struct{}
	observer OverloadObserver
}

func newLimitedListener(listener net.Listener, limit int, observer OverloadObserver) net.Listener {
	return &limitedListener{Listener: listener, sem: make(chan struct{}, limit), observer: observer}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.sem <- struct{}{}:
			return &limitedConn{Conn: conn, release: func() { <-l.sem }}, nil
		default:
			if l.observer != nil {
				l.observer.Overload("tcp_connections")
			}
			_ = conn.Close()
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
