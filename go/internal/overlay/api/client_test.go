package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/platform/apiclient"
)

func TestNewTimeouts(t *testing.T) {
	t.Setenv("QUESTS_API", "http://127.0.0.1:9")
	t.Setenv("QUESTS_API_TIMEOUT", "")
	c := New(config.Config{})
	if c.HTTP.Timeout.Milliseconds() != 1500 {
		t.Fatalf("local timeout: %v", c.HTTP.Timeout)
	}
	t.Setenv("QUESTS_API", "http://192.168.1.11:8765")
	c = New(config.Config{})
	if c.HTTP.Timeout.Seconds() != 5 {
		t.Fatalf("remote timeout: %v", c.HTTP.Timeout)
	}
	t.Setenv("QUESTS_API_TIMEOUT", "8")
	c = New(config.Config{APIBase: "http://example"})
	if c.HTTP.Timeout.Seconds() != 8 {
		t.Fatalf("env timeout: %v", c.HTTP.Timeout)
	}
}

func TestFetchHelpers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/quests", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]Quest{{ID: 1, Title: "a", Status: "active"}})
	})
	mux.HandleFunc("/api/categories", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]Category{{Slug: "work", Label: "Work"}})
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") != "3" {
			t.Fatalf("since=%q", r.URL.Query().Get("since"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"revision": 5,
			"events":   []map[string]any{{"kind": "ping"}},
		})
	})
	hb := 0
	mux.HandleFunc("/api/health/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		hb++
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	base := apiclient.NewClient(srv.URL, true)
	c := &Client{Client: base}

	quests, err := c.FetchQuests()
	if err != nil || len(quests) != 1 || quests[0].Title != "a" {
		t.Fatalf("quests=%v err=%v", quests, err)
	}
	cats, err := c.FetchCategories()
	if err != nil || len(cats) != 1 || cats[0].Slug != "work" {
		t.Fatalf("cats=%v err=%v", cats, err)
	}
	rev, events, err := c.FetchEvents(3)
	if err != nil || rev != 5 || len(events) != 1 {
		t.Fatalf("rev=%d events=%v err=%v", rev, events, err)
	}
	c.PostHeartbeat("test")
	if hb != 1 {
		t.Fatalf("heartbeat calls=%d", hb)
	}
}

func TestFetchEventsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(srv.Close)
	c := &Client{Client: apiclient.NewClient(srv.URL, true)}
	if _, _, err := c.FetchEvents(1); err == nil {
		t.Fatal("expected error")
	}
}
