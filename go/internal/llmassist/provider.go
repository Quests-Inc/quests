package llmassist

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Provider is a choice offered to the account owner. All of them speak the
// OpenAI-style chat-completions API with JSON-schema output; "custom" takes any
// such endpoint (a self-hosted gateway, OpenRouter-compatible, a local proxy…).
type Provider struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	// EditableBase: the user types the address (custom).
	EditableBase bool `json:"editable_base"`
}

// Providers lists what the settings screen offers.
var Providers = []Provider{
	{ID: "groq", Label: "Groq", BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "openai/gpt-oss-20b"},
	{ID: "openai", Label: "OpenAI", BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini"},
	{ID: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "openai/gpt-4o-mini"},
	{ID: "custom", Label: "Другой (OpenAI-совместимый)", EditableBase: true},
}

// ProviderByID returns the preset, or false.
func ProviderByID(id string) (Provider, bool) {
	for _, p := range Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// ErrBaseURL is returned for an endpoint address that may not be used.
var ErrBaseURL = errors.New("адрес провайдера: нужен https и публичный адрес (не localhost и не внутренняя сеть)")

// blockedIP reports addresses a user-supplied endpoint must never reach: this
// server's own network (loopback, LAN, link-local incl. cloud metadata, CGNAT/Tailscale).
func blockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if cgnat := netip.MustParsePrefix("100.64.0.0/10"); cgnat.Contains(ip) {
		return true
	}
	return false
}

// CheckBaseURL validates a user-supplied endpoint: https, no credentials in the URL, a host
// that resolves only to public addresses. Returns the cleaned address.
func CheckBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrBaseURL
	}
	host := u.Hostname()
	if ip, perr := netip.ParseAddr(host); perr == nil {
		if blockedIP(ip) {
			return "", ErrBaseURL
		}
		return raw, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", ErrBaseURL
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("адрес провайдера не разрешается: %s", host)
	}
	for _, ip := range ips {
		if a, ok := netip.AddrFromSlice(ip); ok && blockedIP(a) {
			return "", ErrBaseURL
		}
	}
	return raw, nil
}

// publicIP resolves host and returns an address to connect to, refusing the host when ANY of
// its addresses is internal (a name that also points inward is not trusted).
func publicIP(host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if blockedIP(ip) {
			return netip.Addr{}, ErrBaseURL
		}
		return ip, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return netip.Addr{}, fmt.Errorf("адрес провайдера не разрешается: %s", host)
	}
	var pick netip.Addr
	for _, raw := range ips {
		a, ok := netip.AddrFromSlice(raw)
		if !ok || blockedIP(a) {
			return netip.Addr{}, ErrBaseURL
		}
		if !pick.IsValid() || (a.Unmap().Is4() && !pick.Unmap().Is4()) {
			pick = a.Unmap()
		}
	}
	return pick, nil
}

// do sends req with the client for s. A user-chosen endpoint is resolved right here and the
// request goes to the address that was checked (TLS and the Host header keep the real name):
// connecting by name later — directly, or through a proxy that resolves it itself — would let
// DNS be pointed at this server's own network between the check and the connection.
func do(s Settings, req *http.Request) (*http.Response, error) {
	c := clientFor(s)
	if s.Untrusted {
		host := req.URL.Hostname()
		ip, err := publicIP(host)
		if err != nil {
			return nil, err
		}
		port := req.URL.Port()
		if port == "" {
			port = "443"
		}
		req.Host = req.URL.Host
		req.URL.Host = net.JoinHostPort(ip.String(), port)
		c.Transport.(*http.Transport).TLSClientConfig = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	return httpDo(c, req)
}

// httpDo is the single place a request leaves; tests replace it.
var httpDo = func(c *http.Client, r *http.Request) (*http.Response, error) { return c.Do(r) }

// clientFor builds the HTTP client for a request. Settings the account owner typed in
// (Untrusted) get a dialer that refuses internal addresses at connect time — so a
// name that resolved to something public when saved cannot be re-pointed inward later —
// and never follow redirects.
func clientFor(s Settings) *http.Client {
	tr := &http.Transport{}
	if s.Proxy != "" {
		if pu, err := url.Parse(s.Proxy); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	c := &http.Client{Timeout: s.Timeout, Transport: tr}
	if !s.Untrusted {
		return c
	}
	if s.Proxy == "" {
		d := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBaseURL
			}
			if ip, perr := netip.ParseAddr(host); perr != nil || blockedIP(ip) {
				return ErrBaseURL
			}
			return nil
		}}
		tr.DialContext = d.DialContext
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// Ping checks that the endpoint answers and the key is accepted (GET /models).
func Ping(ctx context.Context, s Settings) error {
	if s.APIKey == "" {
		return llmErrf("ключ не задан")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("User-Agent", "quests-bot/1.0")
	resp, err := do(s, req)
	if err != nil {
		return llmErrf("не удалось связаться с провайдером: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return llmErrf("провайдер не принял ключ (HTTP %d)", resp.StatusCode)
	case resp.StatusCode >= 300:
		return llmErrf("провайдер ответил HTTP %d", resp.StatusCode)
	}
	return nil
}
