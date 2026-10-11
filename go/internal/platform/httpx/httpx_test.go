package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientKeyIgnoresPortAndGarbage(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.RemoteAddr = "192.168.1.5:40001"
	a := ClientKey(r)
	r.RemoteAddr = "192.168.1.5:40002"
	if b := ClientKey(r); a != b || a != "192.168.1.5" {
		t.Fatalf("key depends on the source port: %q vs %q", a, b)
	}
	r.Header.Set("X-Forwarded-For", "not an ip, 1.2.3.4")
	if got := ClientKey(r); got != "192.168.1.5" {
		t.Fatalf("a malformed X-Forwarded-For must not become the key, got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := ClientKey(r); got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
}
