package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/valirum/quests/go/internal/platform/health"
)

func TestProbeRecordsWebDAVAndDisabledClam(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, "", 0)
	s.Health = health.New()
	s.probeOnce(t.Context())
	snap := s.Health.Snapshot()
	davc, _ := snap["webdav"].(map[string]any)
	if davc["status"] != "ok" {
		t.Fatalf("webdav = %v", davc)
	}
	clam, _ := snap["clamav"].(map[string]any)
	if clam["status"] != "disabled" {
		t.Fatalf("clamav = %v, want disabled", clam["status"])
	}
}

func TestProbeWebDAVOffline(t *testing.T) {
	s := newAttachmentServer(t, "http://127.0.0.1:1", "", 0)
	s.Health = health.New()
	s.probeOnce(t.Context())
	snap := s.Health.Snapshot()
	davc, _ := snap["webdav"].(map[string]any)
	if davc["status"] != "offline" {
		t.Fatalf("webdav = %v, want offline", davc)
	}
}

func TestGetHealthIncludesProbes(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	s.Health = health.New()
	s.probeOnce(t.Context())
	w := doJSON(t, s.Handler(), "GET", "/api/health", nil, nil)
	if w.Code != 200 {
		t.Fatalf("health = %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	comps, _ := body["components"].(map[string]any)
	dav, _ := comps["webdav"].(map[string]any)
	if dav["status"] != "disabled" {
		t.Fatalf("health webdav = %v", dav)
	}
}
