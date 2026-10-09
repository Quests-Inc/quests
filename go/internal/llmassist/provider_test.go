package llmassist

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestBlockedIP(t *testing.T) {
	for _, bad := range []string{"127.0.0.1", "::1", "10.0.0.5", "192.168.1.11", "172.16.0.1", "169.254.169.254",
		"100.64.1.1", "100.127.255.254", "0.0.0.0", "224.0.0.1", "::ffff:127.0.0.1", "fe80::1", "fc00::1"} {
		if !blockedIP(netip.MustParseAddr(bad)) {
			t.Errorf("%s must be blocked", bad)
		}
	}
	for _, ok := range []string{"8.8.8.8", "104.18.2.2", "2606:4700::1111"} {
		if blockedIP(netip.MustParseAddr(ok)) {
			t.Errorf("%s must be allowed", ok)
		}
	}
}

func TestCheckBaseURL(t *testing.T) {
	for _, bad := range []string{
		"http://api.example.com/v1", "https://localhost/v1", "https://127.0.0.1:8765/v1", "https://[::1]/v1",
		"https://192.168.1.11:3001", "https://169.254.169.254/latest", "https://user:pw@api.example.com/v1",
		"https://x.localhost/v1", "https://box.local/v1", "ftp://x/", "", "not a url", "https://api.example.com/v1?x=1",
	} {
		if _, err := CheckBaseURL(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if got, err := CheckBaseURL("https://8.8.8.8/v1/"); err != nil || got != "https://8.8.8.8/v1" {
		t.Errorf("public literal: %q %v", got, err)
	}
}

// A user-chosen endpoint must not reach the machine's own network even when the
// address check at save time was passed (DNS can change); the operator's may.
func TestUntrustedClientCannotDialLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	trusted := Settings{BaseURL: srv.URL, APIKey: "k", Timeout: 5 * time.Second}
	if err := Ping(context.Background(), trusted); err != nil {
		t.Fatalf("operator settings must work against a local endpoint: %v", err)
	}
	untrusted := trusted
	untrusted.Untrusted = true
	if err := Ping(context.Background(), untrusted); err == nil {
		t.Fatal("a user-supplied endpoint reached loopback")
	}
}

func TestUntrustedClientRefusesRedirects(t *testing.T) {
	if c := clientFor(Settings{Untrusted: true, Timeout: time.Second}); c.CheckRedirect == nil {
		t.Fatal("a user-chosen endpoint must not be able to bounce the request elsewhere")
	}
	if c := clientFor(Settings{Timeout: time.Second}); c.CheckRedirect != nil {
		t.Fatal("operator settings keep the default redirect policy")
	}
}

func TestWithUserPresetsAndCustom(t *testing.T) {
	base := Settings{Timeout: time.Minute, Temperature: 0.1, APIKey: "server-key"}
	g := base.WithUser("groq", "ignored", "", "user-key")
	if g.APIKey != "user-key" || g.BaseURL != "https://api.groq.com/openai/v1" || g.Model != "openai/gpt-oss-20b" || !g.Untrusted {
		t.Fatalf("groq preset: %+v", g)
	}
	c := base.WithUser("custom", "https://gw.example.com/v1/", "my-model", "k")
	if c.BaseURL != "https://gw.example.com/v1" || c.Model != "my-model" {
		t.Fatalf("custom: %+v", c)
	}
	if base.APIKey != "server-key" {
		t.Fatal("WithUser must not modify the receiver")
	}
}

func TestPublicIPRefusesInternalAndLiteral(t *testing.T) {
	for _, bad := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "100.100.1.1"} {
		if _, err := publicIP(bad); err == nil {
			t.Errorf("%s must be refused", bad)
		}
	}
	if ip, err := publicIP("8.8.4.4"); err != nil || ip.String() != "8.8.4.4" {
		t.Errorf("public literal: %v %v", ip, err)
	}
}

// A request to a user-chosen endpoint is rewritten to the checked address, keeping the real
// name for TLS and Host; one whose name resolves inward never leaves the machine.
func TestDoPinsTheCheckedAddress(t *testing.T) {
	var seenHost, seenURLHost string
	prev := httpDo
	httpDo = func(c *http.Client, r *http.Request) (*http.Response, error) {
		seenHost, seenURLHost = r.Host, r.URL.Host
		return nil, errStopHere
	}
	defer func() { httpDo = prev }()
	req, _ := http.NewRequest(http.MethodGet, "https://8.8.4.4:8443/v1/models", nil)
	_, _ = do(Settings{Untrusted: true, Timeout: time.Second}, req)
	if seenURLHost != "8.8.4.4:8443" || seenHost != "8.8.4.4:8443" {
		t.Fatalf("literal kept: host=%q url=%q", seenHost, seenURLHost)
	}
	req2, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1/v1", nil)
	called := false
	httpDo = func(c *http.Client, r *http.Request) (*http.Response, error) { called = true; return nil, errStopHere }
	if _, err := do(Settings{Untrusted: true, Timeout: time.Second}, req2); err == nil || called {
		t.Fatalf("an internal address must be refused before any request (called=%v, err=%v)", called, err)
	}
}

var errStopHere = errors.New("stop")
