package cluster

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestNewAuthorityCreatesUsableCA(t *testing.T) {
	authority, err := NewAuthority("cluster-a", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(authority.CertificatePEM)
	if block == nil {
		t.Fatal("missing CA certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !certificate.IsCA {
		t.Fatalf("certificate = %#v, %v", certificate, err)
	}
	if _, err := parsePrivateKey(authority.PrivateKeyPEM); err != nil {
		t.Fatal(err)
	}
}

func TestJoinTokensAreOpaqueAndHashed(t *testing.T) {
	raw, digest, err := NewJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 || len(digest) != 32 {
		t.Fatalf("unexpected token lengths: raw=%d digest=%d", len(raw), len(digest))
	}
	if string(digest) != string(TokenDigest(raw)) {
		t.Fatal("token digest is not deterministic")
	}
}

func TestIssueNodeIdentityBindsControlAddress(t *testing.T) {
	authority, err := NewAuthority("cluster-a", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM, keyPEM, err := IssueNodeIdentity(authority, "cluster-a", "node-a", "127.0.0.1:9443", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certificatePEM)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || len(certificate.IPAddresses) != 1 || !certificate.IPAddresses[0].Equal([]byte{127, 0, 0, 1}) {
		t.Fatalf("certificate = %#v, %v", certificate, err)
	}
	if _, err := parsePrivateKey(keyPEM); err != nil {
		t.Fatal(err)
	}
}
