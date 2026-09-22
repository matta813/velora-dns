package cluster

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func Join(ctx context.Context, bundle JoinBundle, nodeID, nodeName, controlAddress string) (State, error) {
	if bundle.Token == "" || bundle.LeaderAddress == "" || len(bundle.CACertificate) == 0 || !bundle.ExpiresAt.After(time.Now()) {
		return State{}, fmt.Errorf("invalid or expired join bundle")
	}
	csr, key, err := NewCSR(nodeID, controlAddress)
	if err != nil {
		return State{}, err
	}
	host, _, err := net.SplitHostPort(bundle.LeaderAddress)
	if err != nil {
		return State{}, fmt.Errorf("invalid leader address: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle.CACertificate) {
		return State{}, fmt.Errorf("invalid cluster CA certificate")
	}
	payload, err := json.Marshal(JoinRequest{Token: bundle.Token, NodeID: nodeID, NodeName: nodeName, ControlAddress: controlAddress, CSR: csr})
	if err != nil {
		return State{}, err
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: strings.Trim(host, "[]")}}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+bundle.LeaderAddress+"/control/v1/join", bytes.NewReader(payload))
	if err != nil {
		return State{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return State{}, fmt.Errorf("contact cluster leader: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return State{}, fmt.Errorf("cluster leader rejected join")
	}
	var accepted JoinResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&accepted); err != nil {
		return State{}, fmt.Errorf("decode join response: %w", err)
	}
	certificate, err := certificateFromPEM(accepted.Certificate)
	if err != nil {
		return State{}, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(accepted.CACertificate)
	if _, err = certificate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: time.Now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return State{}, fmt.Errorf("verify joined node certificate: %w", err)
	}
	return State{ClusterID: accepted.ClusterID, NodeID: nodeID, NodeName: nodeName, ControlAddress: controlAddress, Role: "voter", CACertificate: accepted.CACertificate, Certificate: accepted.Certificate, PrivateKey: key, CreatedAt: time.Now().UTC()}, nil
}

// JoinBundle is shown exactly once to an administrator. Its token is a bearer
// credential, while the embedded CA pins the leader during bootstrap.
type JoinBundle struct {
	LeaderAddress string    `json:"leader_address"`
	CACertificate []byte    `json:"ca_certificate"`
	Token         string    `json:"token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func NewCSR(nodeID, controlAddress string) (csrPEM, privateKeyPEM []byte, err error) {
	host, _, err := net.SplitHostPort(controlAddress)
	if err != nil || host == "" {
		return nil, nil, fmt.Errorf("invalid control address %q", controlAddress)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate node key: %w", err)
	}
	request := &x509.CertificateRequest{Subject: pkix.Name{CommonName: nodeID}}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		request.IPAddresses = []net.IP{ip}
	} else {
		request.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, request, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate request: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal node key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

func SignCSR(authority Authority, clusterID, nodeID, controlAddress string, csrPEM []byte, now time.Time) ([]byte, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, fmt.Errorf("invalid certificate request PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate request: %w", err)
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("verify certificate request: %w", err)
	}
	host, _, err := net.SplitHostPort(controlAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid control address: %w", err)
	}
	if csr.Subject.CommonName != nodeID {
		return nil, fmt.Errorf("certificate request node ID mismatch")
	}
	if len(csr.DNSNames)+len(csr.IPAddresses) != 1 {
		return nil, fmt.Errorf("certificate request must contain exactly one address")
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if len(csr.IPAddresses) != 1 || !csr.IPAddresses[0].Equal(ip) {
			return nil, fmt.Errorf("certificate request address mismatch")
		}
	} else if len(csr.DNSNames) != 1 || csr.DNSNames[0] != host {
		return nil, fmt.Errorf("certificate request address mismatch")
	}
	caBlock, _ := pem.Decode(authority.CertificatePEM)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}
	caKey, err := parsePrivateKey(authority.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: nodeID, Organization: []string{"Velora " + clusterID}}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, DNSNames: csr.DNSNames, IPAddresses: csr.IPAddresses}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, csr.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("sign node certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}
