package cluster

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestMutualTLSConfigsUseClusterAuthority(t *testing.T) {
	now := time.Now()
	authority, err := NewAuthority("cluster-a", now)
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := IssueNodeIdentity(authority, "cluster-a", "node-a", "127.0.0.1:9443", now)
	if err != nil {
		t.Fatal(err)
	}
	state := State{CACertificate: authority.CertificatePEM, Certificate: cert, PrivateKey: key}
	server, err := ServerTLSConfig(state)
	if err != nil || server.ClientAuth == 0 || server.MinVersion != 0x0304 {
		t.Fatalf("server=%#v err=%v", server, err)
	}
	client, err := ClientTLSConfig(state, "127.0.0.1")
	if err != nil || client.ServerName != "127.0.0.1" {
		t.Fatalf("client=%#v err=%v", client, err)
	}
	bootstrap, err := BootstrapTLSConfig(state)
	if err != nil || bootstrap.ClientAuth != tls.NoClientCert {
		t.Fatalf("bootstrap=%#v err=%v", bootstrap, err)
	}
}
