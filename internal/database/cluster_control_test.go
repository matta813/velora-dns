package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cluster"
)

func TestClusterStateAndSingleUseJoinToken(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "cluster.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := cluster.State{ClusterID: "cluster-1", NodeID: "node-1", NodeName: "primary", ControlAddress: "127.0.0.1:9443", Role: "leader", CACertificate: []byte("ca"), Certificate: []byte("cert"), PrivateKey: []byte("key"), CreatedAt: time.Now().UTC()}
	if err := store.SaveClusterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetClusterState(ctx)
	if err != nil || loaded.ClusterID != state.ClusterID || string(loaded.PrivateKey) != "key" {
		t.Fatalf("state = %#v, %v", loaded, err)
	}
	raw, digest, err := cluster.NewJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CreateClusterJoinToken(ctx, digest, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = store.ConsumeClusterJoinToken(ctx, cluster.TokenDigest(raw)); err != nil {
		t.Fatal(err)
	}
	if err = store.ConsumeClusterJoinToken(ctx, cluster.TokenDigest(raw)); err == nil {
		t.Fatal("join token was reusable")
	}
}
