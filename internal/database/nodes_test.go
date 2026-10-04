package database

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/node"
)

func TestNodeRegistrationUpdatesAndReads(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "nodes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := node.Node{ID: "local", Name: "Primary", Address: "127.0.0.1:9999", Capabilities: []string{"dns", "zones"}, Status: "unknown", LastSeenAt: time.Now().UTC().Truncate(time.Second)}
	if err := db.SaveNode(ctx, first); err != nil {
		t.Fatal(err)
	}
	first.Name = "Updated"
	first.Status = "healthy"
	first.LastSeenAt = first.LastSeenAt.Add(time.Minute)
	if err := db.SaveNode(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetNode(ctx, first.ID)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("get node: got %+v, err %v", got, err)
	}
	nodes, err := db.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || !reflect.DeepEqual(nodes[0], first) {
		t.Fatalf("list nodes: got %+v, err %v", nodes, err)
	}
}
