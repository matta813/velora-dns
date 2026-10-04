package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAuditOutcomesFilteringAndRetention(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	user, err := store.CreateUser(ctx, "admin", "correct horse battery staple", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO audit_events(action,occurred_at) VALUES('old','2000-01-01 00:00:00')"); err != nil {
		t.Fatal(err)
	}
	id, err := store.BeginAudit(ctx, user.ID, "admin", "PUT", "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteAudit(ctx, id, 200); err != nil {
		t.Fatal(err)
	}
	failedID, err := store.BeginAudit(ctx, user.ID, "admin", "PUT", "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteAudit(ctx, failedID, 400); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListAudit(ctx, AuditFilter{Result: "success", Actor: "admin", Action: "PUT", Limit: 10})
	if err != nil || len(events) != 1 || events[0].ID != id || events[0].Role != "admin" || events[0].StatusCode != 200 {
		t.Fatalf("filtered audit: %+v (%v)", events, err)
	}
	events, err = store.ListAudit(ctx, AuditFilter{Before: failedID, Limit: 10})
	if err != nil || len(events) != 1 || events[0].ID != id {
		t.Fatalf("pagination or retention: %+v (%v)", events, err)
	}
}
