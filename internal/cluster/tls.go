package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// ServerTLSConfig enforces mutual TLS for every control-plane request.
func ServerTLSConfig(state State) (*tls.Config, error) {
	certificate, pool, err := tlsMaterial(state)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}, nil
}

// ClientTLSConfig verifies that the peer certificate is signed by this
// cluster's CA and matches the advertised peer name or IP address.
func ClientTLSConfig(state State, serverName string) (*tls.Config, error) {
	certificate, pool, err := tlsMaterial(state)
	if err != nil {
		return nil, err
	}
	if serverName == "" {
		return nil, fmt.Errorf("control-plane server name is required")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: pool, ServerName: serverName}, nil
}

func tlsMaterial(state State) (tls.Certificate, *x509.CertPool, error) {
	certificate, err := tls.X509KeyPair(state.Certificate, state.PrivateKey)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("load node identity: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(state.CACertificate) {
		return tls.Certificate{}, nil, fmt.Errorf("load cluster CA certificate")
	}
	return certificate, pool, nil
}

func certificateFromPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
