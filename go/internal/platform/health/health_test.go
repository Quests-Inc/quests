package health

import (
	"testing"
	"time"
)

func TestSnapshotHeartbeatAndProbe(t *testing.T) {
	r := New()
	snap := r.Snapshot()
	overlay, _ := snap["overlay"].(map[string]any)
	if overlay["status"] != "offline" {
		t.Fatalf("overlay before heartbeat = %v", overlay["status"])
	}
	dav, _ := snap["webdav"].(map[string]any)
	if dav["status"] != "disabled" {
		t.Fatalf("webdav before probe = %v, want disabled", dav["status"])
	}

	r.Record("overlay", "tab")
	r.SetProbe("webdav", "ok", "")
	r.SetProbe("clamav", "offline", "connection refused")
	r.SetProbe("db", "warn", "БД alembic=ffff новее API")
	snap = r.Snapshot()
	overlay, _ = snap["overlay"].(map[string]any)
	if overlay["status"] != "ok" {
		t.Fatalf("overlay after heartbeat = %v", overlay["status"])
	}
	dav, _ = snap["webdav"].(map[string]any)
	if dav["status"] != "ok" {
		t.Fatalf("webdav = %v", dav["status"])
	}
	clam, _ := snap["clamav"].(map[string]any)
	if clam["status"] != "offline" {
		t.Fatalf("clamav = %v", clam["status"])
	}
	if clam["detail"] != "connection refused" {
		t.Fatalf("clamav detail = %v", clam["detail"])
	}
	dbComp, _ := snap["db"].(map[string]any)
	if dbComp["status"] != "warn" {
		t.Fatalf("db = %v", dbComp["status"])
	}
}

func TestHeartbeatGoesStale(t *testing.T) {
	r := New()
	r.mu.Lock()
	r.seen["telegram"] = heartbeat{at: time.Now().UTC().Add(-time.Minute), detail: "old"}
	r.mu.Unlock()
	snap := r.Snapshot()
	tg, _ := snap["telegram"].(map[string]any)
	if tg["status"] != "offline" {
		t.Fatalf("stale telegram = %v, want offline", tg["status"])
	}
}
