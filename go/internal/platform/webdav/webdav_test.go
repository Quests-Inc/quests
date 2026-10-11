package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPingTreats404AsUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead && r.Method != http.MethodOptions {
			t.Errorf("method = %s", r.Method)
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "", "")
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping 404: %v", err)
	}
}

func TestPingRejectsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "u", "p")
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error on 401")
	}
}

func TestPingUnconfigured(t *testing.T) {
	c := New("", "", "")
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected not configured")
	}
}
