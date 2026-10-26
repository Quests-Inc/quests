package egress

import (
	"compress/gzip"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestPolicyFromEnv(t *testing.T) {
	p, err := PolicyFromEnv(env(nil))
	if err != nil || p.Mode != ModeOff || p.Enabled() || !p.Ports[443] || p.MaxBody != DefaultMaxBody {
		t.Fatalf("default: %+v %v", p, err)
	}
	p, err = PolicyFromEnv(env(map[string]string{
		"QUESTS_EGRESS_MODE": "allowlist", "QUESTS_EGRESS_ALLOW": "api.open-meteo.com, *.example.org",
		"QUESTS_EGRESS_ALLOW_NETS": "192.168.1.5, 10.1.0.0/16", "QUESTS_EGRESS_PORTS": "443,8443", "QUESTS_EGRESS_TIMEOUT": "3",
	}))
	if err != nil || !p.Enabled() || len(p.AllowNets) != 2 || !p.Ports[8443] || p.Ports[80] || p.Timeout != 3*time.Second {
		t.Fatalf("filled: %+v %v", p, err)
	}
	for _, bad := range []map[string]string{
		{"QUESTS_EGRESS_MODE": "everything"},
		{"QUESTS_EGRESS_MODE": "allowlist"},
		{"QUESTS_EGRESS_ALLOW": "bad_host"},
		{"QUESTS_EGRESS_ALLOW_NETS": "not-an-ip"},
		{"QUESTS_EGRESS_PORTS": "0"},
		{"QUESTS_EGRESS_RATE": "-1"},
	} {
		if _, err := PolicyFromEnv(env(bad)); err == nil {
			t.Errorf("%v must be refused", bad)
		}
	}
}

func TestHostMatchAndNormalize(t *testing.T) {
	p := Policy{Mode: ModeAllowlist, Allow: []string{"api.example.org", "*.cdn.example.org"}}
	for host, want := range map[string]bool{
		"api.example.org": true, "x.cdn.example.org": true, "cdn.example.org": false,
		"evil.org": false, "api.example.org.evil.org": false, "xapi.example.org": false,
	} {
		if p.HostAllowed(host) != want {
			t.Errorf("%s: %v", host, !want)
		}
	}
	for _, bad := range []string{"", "2130706433", "127.1", "0x7f.1", "exa mple.com", "пример.рф", "a_b.com", "-a.com"} {
		if _, err := NormalizeHost(bad); err == nil {
			t.Errorf("host %q must be refused", bad)
		}
	}
	if h, err := NormalizeHost("API.Example.ORG."); err != nil || h != "api.example.org" {
		t.Errorf("normalise: %q %v", h, err)
	}
}

func TestBlocked(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.11", "100.64.0.1", "169.254.169.254", "0.0.0.0", "224.0.0.1",
		"255.255.255.255", "::1", "::", "fe80::1", "fc00::1", "fd12::1", "::ffff:127.0.0.1", "::ffff:192.168.0.1",
		"64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe", "2002:7f00:1::", "2002:c0a8:101::", "2001:0:4136:e378::1", "100::1",
	} {
		if !Blocked(netip.MustParseAddr(s), nil) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111", "::ffff:8.8.8.8", "64:ff9b::808:808", "2002:808:808::"} {
		if Blocked(netip.MustParseAddr(s), nil) {
			t.Errorf("%s is public", s)
		}
	}
	allow := []netip.Prefix{netip.MustParsePrefix("192.168.1.5/32")}
	if Blocked(netip.MustParseAddr("192.168.1.5"), allow) || !Blocked(netip.MustParseAddr("192.168.1.6"), allow) {
		t.Error("an exception covers only what the administrator named")
	}
	if Blocked(netip.MustParseAddr("::ffff:192.168.1.5"), allow) {
		t.Error("a mapped address is judged as its IPv4")
	}
}

// listen starts a server on the given loopback address.
func listen(t *testing.T, ip string, h http.Handler) *httptest.Server {
	t.Helper()
	l, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Skipf("no %s: %v", ip, err)
	}
	s := httptest.NewUnstartedServer(h)
	s.Listener = l
	s.Start()
	t.Cleanup(s.Close)
	return s
}

func policyFor(ports ...string) Policy {
	p, _ := PolicyFromEnv(env(map[string]string{"QUESTS_EGRESS_MODE": "public", "QUESTS_EGRESS_PORTS": strings.Join(ports, ",")}))
	return p
}

func portOf(s *httptest.Server) string {
	_, p, _ := net.SplitHostPort(s.Listener.Addr().String())
	return p
}

func TestClientRefusesInternalAddresses(t *testing.T) {
	srv := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
	var log []Entry
	c := New(policyFor(portOf(srv)), func(e Entry) { log = append(log, e) })
	_, err := c.Do(context.Background(), Request{Workspace: 1, URL: srv.URL})
	var d *Denied
	if !errors.As(err, &d) {
		t.Fatalf("loopback must be denied, got %v", err)
	}
	if len(log) != 1 || !strings.HasPrefix(log[0].Result, "denied") {
		t.Fatalf("log = %+v", log)
	}
	// The administrator may open that one address, and only it.
	p := policyFor(portOf(srv))
	p.AllowNets = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	res, err := New(p, nil).Do(context.Background(), Request{Workspace: 1, URL: srv.URL})
	if err != nil || string(res.Body) != "hi" {
		t.Fatalf("an allowed net: %v %v", res, err)
	}
}

func TestClientChecksEveryRedirect(t *testing.T) {
	inner := listen(t, "127.0.0.2", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secret")) }))
	outer := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL, http.StatusFound)
	}))
	p := policyFor(portOf(outer), portOf(inner))
	p.AllowNets = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	_, err := New(p, nil).Do(context.Background(), Request{Workspace: 1, URL: outer.URL})
	var d *Denied
	if !errors.As(err, &d) {
		t.Fatalf("a redirect to an internal address must be denied: %v", err)
	}
}

func TestClientRedirectLimit(t *testing.T) {
	var self string
	srv := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, self, http.StatusFound) }))
	self = srv.URL
	p := policyFor(portOf(srv))
	p.AllowNets = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	if _, err := New(p, nil).Do(context.Background(), Request{Workspace: 1, URL: srv.URL}); err == nil {
		t.Fatal("a loop of redirects must stop")
	}
}

func TestClientURLRules(t *testing.T) {
	p, _ := PolicyFromEnv(env(map[string]string{"QUESTS_EGRESS_MODE": "allowlist", "QUESTS_EGRESS_ALLOW": "api.example.org"}))
	c := New(p, nil)
	for _, u := range []string{
		"ftp://api.example.org/", "file:///etc/passwd", "http://api.example.org@evil.example.com/", "http://user:pw@api.example.org/",
		"https://evil.example.com/", "https://api.example.org.evil.com/", "https://api.example.org:8443/", "http://2130706433/", "http://[::1]/",
		"http://127.0.0.1/", "https://api.example.org:22/", "//api.example.org/", "api.example.org/x",
	} {
		_, err := c.Do(context.Background(), Request{Workspace: 1, URL: u})
		var d *Denied
		if !errors.As(err, &d) {
			t.Errorf("%s must be denied, got %v", u, err)
		}
	}
	off := New(Policy{Mode: ModeOff, Ports: map[int]bool{443: true}}, nil)
	if _, err := off.Do(context.Background(), Request{Workspace: 1, URL: "https://api.example.org/"}); err == nil {
		t.Error("mode off denies everything")
	}
}

func TestClientLimits(t *testing.T) {
	big := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 3000)))
	}))
	zip := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(strings.Repeat("a", 50000))) // small on the wire, large unpacked
		_ = gz.Close()
	}))
	slow := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { time.Sleep(2 * time.Second) }))
	p := policyFor(portOf(big), portOf(zip), portOf(slow))
	p.AllowNets = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	p.MaxBody, p.Timeout = 2048, 500*time.Millisecond
	c := New(p, nil)
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: big.URL}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("big: %v", err)
	}
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: zip.URL}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("gzip bomb: %v", err)
	}
	start := time.Now()
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: slow.URL}); err == nil || time.Since(start) > 1500*time.Millisecond {
		t.Errorf("slow: %v after %v", err, time.Since(start))
	}
}

func TestClientRateAndConcurrency(t *testing.T) {
	block := make(chan struct{})
	srv := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { <-block; _, _ = w.Write([]byte("ok")) }))
	p := policyFor(portOf(srv))
	p.AllowNets = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	p.RatePerHour, p.Concurrency, p.Timeout = 3, 1, 3*time.Second
	c := New(p, nil)
	done := make(chan error, 1)
	go func() { _, err := c.Do(context.Background(), Request{Workspace: 1, URL: srv.URL}); done <- err }()
	time.Sleep(200 * time.Millisecond)
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: srv.URL}); !errors.Is(err, ErrBusy) {
		t.Errorf("second at once: %v", err)
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: srv.URL}); !errors.Is(err, ErrRate) {
		t.Errorf("the hour is used: %v", err)
	}
	if _, err := c.Do(context.Background(), Request{Workspace: 2, URL: srv.URL}); errors.Is(err, ErrRate) {
		t.Errorf("another workspace has its own hour: %v", err)
	}
}

func TestProxyIsTheAdministratorsAndNeedsAnAllowlist(t *testing.T) {
	if _, err := PolicyFromEnv(env(map[string]string{"QUESTS_EGRESS_MODE": "public", "QUESTS_EGRESS_PROXY": "http://127.0.0.1:2080"})); err == nil {
		t.Fatal("a proxy with public mode must be refused: it resolves names where the server cannot look")
	}
	for _, bad := range []string{"socks5://127.0.0.1:1080", "http://user:pw@127.0.0.1:2080", "not a url", "http://"} {
		if _, err := PolicyFromEnv(env(map[string]string{"QUESTS_EGRESS_MODE": "allowlist", "QUESTS_EGRESS_ALLOW": "api.example.org", "QUESTS_EGRESS_PROXY": bad})); err == nil {
			t.Errorf("proxy %q must be refused", bad)
		}
	}
	var gotHost string
	proxy := listen(t, "127.0.0.1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		_, _ = w.Write([]byte("via proxy"))
	}))
	p, err := PolicyFromEnv(env(map[string]string{
		"QUESTS_EGRESS_MODE": "allowlist", "QUESTS_EGRESS_ALLOW": "api.example.org", "QUESTS_EGRESS_PROXY": proxy.URL,
	}))
	if err != nil {
		t.Fatal(err)
	}
	c := New(p, nil)
	// The name is the administrator's; the proxy, on this very machine, is let through, and nothing else on it is.
	res, err := c.Do(context.Background(), Request{Workspace: 1, URL: "http://api.example.org/x"})
	if err != nil || string(res.Body) != "via proxy" || gotHost != "api.example.org" {
		t.Fatalf("through the proxy: %v %v host=%q", res, err, gotHost)
	}
	var d *Denied
	if _, err := c.Do(context.Background(), Request{Workspace: 1, URL: "http://evil.example.com/"}); !errors.As(err, &d) {
		t.Fatalf("a host not in the list: %v", err)
	}
}
