package egress

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestSameHostOnlyKeepsProviderCredential(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("PRIVATE-TOKEN") != ""
		w.WriteHeader(200)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	ports := map[int]bool{}
	for _, raw := range []string{source.URL, target.URL} {
		u, _ := url.Parse(raw)
		port, _ := strconv.Atoi(u.Port())
		ports[port] = true
	}
	c := New(Policy{Mode: ModePublic, AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Ports: ports, MaxBody: 1000, Timeout: time.Second, RatePerHour: 10, Concurrency: 1}, nil)
	_, err := c.Do(context.Background(), Request{URL: source.URL, SameHostOnly: true, Header: map[string]string{"PRIVATE-TOKEN": "secret"}})
	var denied *Denied
	if !errors.As(err, &denied) {
		t.Fatalf("redirect accepted: %v", err)
	}
	if leaked {
		t.Fatal("credential reached another origin")
	}
}
