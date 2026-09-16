package dns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

func TestDNSOverTLSAndCertificateReload(t *testing.T) {
	certFile, keyFile := writeCertificate(t, 1)
	config, err := TLSConfig(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	first, err := config.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := StartTLS("127.0.0.1:0", wire.HandlerFunc(answer), config, ServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	query := new(wire.Msg)
	query.SetQuestion("example.test.", wire.TypeA)
	client := &wire.Client{Net: "tcp-tls", TLSConfig: insecureTestTLSConfig(), Timeout: time.Second}
	response, _, err := client.Exchange(query, server.Addresses()[0])
	if err != nil || !response.Response {
		t.Fatalf("DoT exchange: %v %v", response, err)
	}
	forwarded, used, err := (&Forwarder{Upstreams: []string{"tls://" + server.Addresses()[0]}, Timeout: time.Second, TLSConfig: insecureTestTLSConfig()}).Resolve(context.Background(), query)
	if err != nil || !forwarded.Response || used != "tls://"+server.Addresses()[0] {
		t.Fatalf("DoT upstream: %v %s %v", forwarded, used, err)
	}
	writeCertificateAt(t, certFile, keyFile, 2)
	stamp := time.Now().Add(time.Second)
	_ = os.Chtimes(certFile, stamp, stamp)
	_ = os.Chtimes(keyFile, stamp, stamp)
	second, err := config.GetCertificate(nil)
	if err != nil || bytes.Equal(first.Certificate[0], second.Certificate[0]) {
		t.Fatal("certificate was not reloaded")
	}
}

func insecureTestTLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} // #nosec G402 -- ephemeral self-signed test server
}

func writeCertificate(t *testing.T, serial int64) (string, string) {
	t.Helper()
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	writeCertificateAt(t, certFile, keyFile, serial)
	return certFile, keyFile
}

func writeCertificateAt(t *testing.T, certFile, keyFile string, serial int64) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}
