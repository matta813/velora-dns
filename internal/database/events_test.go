package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestSystemEventsDeduplicateAndFilterByRole(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	admin, err := db.CreateUser(ctx, "admin", "long admin password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := db.CreateUser(ctx, "viewer", "long viewer password", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	all := SystemEventInput{Key: "upstream:example", Severity: "warning", Title: "Upstream unavailable", Message: "The upstream is unavailable.", Link: "/", Visibility: "all"}
	if err := db.PublishSystemEvent(ctx, all); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishSystemEvent(ctx, all); err != nil {
		t.Fatal(err)
	}
	adminOnly := SystemEventInput{Key: "config:rollback", Severity: "critical", Title: "Configuration rollback", Message: "Check readiness.", Link: "/settings", Visibility: "admin"}
	if err := db.PublishSystemEvent(ctx, adminOnly); err != nil {
		t.Fatal(err)
	}
	adminPage, err := db.ListSystemEvents(ctx, admin.ID, "admin", 50)
	if err != nil || len(adminPage.Events) != 2 || adminPage.UnreadCount != 2 || adminPage.Events[1].RepeatCount != 2 {
		t.Fatalf("admin page: %+v %v", adminPage, err)
	}
	viewerPage, err := db.ListSystemEvents(ctx, viewer.ID, "viewer", 50)
	if err != nil || len(viewerPage.Events) != 1 || viewerPage.UnreadCount != 1 {
		t.Fatalf("viewer page: %+v %v", viewerPage, err)
	}
	if err := db.MarkSystemEventRead(ctx, viewer.ID, "viewer", adminPage.Events[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("viewer read admin event: %v", err)
	}
	if err := db.MarkSystemEventRead(ctx, viewer.ID, "viewer", viewerPage.Events[0].ID); err != nil {
		t.Fatal(err)
	}
	viewerPage, err = db.ListSystemEvents(ctx, viewer.ID, "viewer", 50)
	if err != nil || viewerPage.UnreadCount != 0 || !viewerPage.Events[0].Read {
		t.Fatalf("read state: %+v %v", viewerPage, err)
	}
	if err := db.PublishSystemEvent(ctx, all); err != nil {
		t.Fatal(err)
	}
	viewerPage, err = db.ListSystemEvents(ctx, viewer.ID, "viewer", 50)
	if err != nil || viewerPage.UnreadCount != 1 || viewerPage.Events[0].RepeatCount != 3 {
		t.Fatalf("deduplicated unread event: %+v %v", viewerPage, err)
	}
	if err := db.PublishSystemEvent(ctx, SystemEventInput{Key: "bad", Severity: "info", Title: "Bad", Link: "//evil.test", Visibility: "all"}); !errors.Is(err, ErrInvalidSystemEvent) {
		t.Fatalf("unsafe event link: %v", err)
	}
}
