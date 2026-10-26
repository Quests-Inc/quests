// Package egress is the one way the server talks to the outside on behalf of a user's script. The server runs no user code, but a
// node that fetches a URL is our code with a user's address in it, so the address is checked where it cannot be argued with: at
// the moment the connection is made, against what the administrator of the server allowed (ADR-0004).
package egress

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Mode is how much of the outside the administrator opens.
type Mode string

const (
	// ModeOff: no node may go out. The default.
	ModeOff Mode = "off"
	// ModePublic: any public address.
	ModePublic Mode = "public"
	// ModeAllowlist: only the hosts in Allow.
	ModeAllowlist Mode = "allowlist"
)

// Policy is the administrator's rules. It is read from the environment of the server when it starts; nothing in the database
// can widen it.
type Policy struct {
	Mode Mode
	// Allow lists hosts: `api.example.org` or `*.example.org` (any subdomain, not the bare domain).
	Allow []string
	// AllowNets are the only way to reach an address that is not public (a service in the local network). Explicit, by the
	// administrator.
	AllowNets []netip.Prefix
	// Proxy is the one proxy every request goes through (a machine that reaches the outside only through one). It is the
	// administrator's address and is allowed to be internal. The proxy resolves the names, where we cannot look, so a proxy needs
	// ModeAllowlist: only hosts the administrator named.
	Proxy       *url.URL
	Ports       map[int]bool
	MaxBody     int64
	Timeout     time.Duration
	RatePerHour int
	Concurrency int
}

// Defaults are the limits when the environment says nothing.
const (
	DefaultMaxBody     = 1 << 20
	DefaultTimeout     = 10 * time.Second
	DefaultRatePerHour = 120
	DefaultConcurrency = 4
)

// PolicyFromEnv reads QUESTS_EGRESS_* through get (os.Getenv in the server).
func PolicyFromEnv(get func(string) string) (Policy, error) {
	p := Policy{
		Mode: ModeOff, Ports: map[int]bool{80: true, 443: true},
		MaxBody: DefaultMaxBody, Timeout: DefaultTimeout, RatePerHour: DefaultRatePerHour, Concurrency: DefaultConcurrency,
	}
	switch m := Mode(strings.ToLower(strings.TrimSpace(get("QUESTS_EGRESS_MODE")))); m {
	case "":
	case ModeOff, ModePublic, ModeAllowlist:
		p.Mode = m
	default:
		return Policy{}, fmt.Errorf("QUESTS_EGRESS_MODE: off, public or allowlist (got %q)", m)
	}
	for _, h := range splitList(get("QUESTS_EGRESS_ALLOW")) {
		h = strings.ToLower(strings.TrimSuffix(h, "."))
		if !validHostPattern(h) {
			return Policy{}, fmt.Errorf("QUESTS_EGRESS_ALLOW: bad host %q", h)
		}
		p.Allow = append(p.Allow, h)
	}
	for _, n := range splitList(get("QUESTS_EGRESS_ALLOW_NETS")) {
		pre, err := parsePrefix(n)
		if err != nil {
			return Policy{}, fmt.Errorf("QUESTS_EGRESS_ALLOW_NETS: %w", err)
		}
		p.AllowNets = append(p.AllowNets, pre)
	}
	if v := splitList(get("QUESTS_EGRESS_PORTS")); len(v) > 0 {
		p.Ports = map[int]bool{}
		for _, s := range v {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 65535 {
				return Policy{}, fmt.Errorf("QUESTS_EGRESS_PORTS: bad port %q", s)
			}
			p.Ports[n] = true
		}
	}
	var err error
	if raw := strings.TrimSpace(get("QUESTS_EGRESS_PROXY")); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return Policy{}, fmt.Errorf("QUESTS_EGRESS_PROXY: an http(s) address without a login (got %q)", raw)
		}
		p.Proxy = u
	}
	if p.MaxBody, err = positive(get("QUESTS_EGRESS_MAX_BODY"), p.MaxBody, "QUESTS_EGRESS_MAX_BODY"); err != nil {
		return Policy{}, err
	}
	secs, err := positive(get("QUESTS_EGRESS_TIMEOUT"), int64(p.Timeout/time.Second), "QUESTS_EGRESS_TIMEOUT")
	if err != nil {
		return Policy{}, err
	}
	p.Timeout = time.Duration(secs) * time.Second
	rate, err := positive(get("QUESTS_EGRESS_RATE"), int64(p.RatePerHour), "QUESTS_EGRESS_RATE")
	if err != nil {
		return Policy{}, err
	}
	p.RatePerHour = int(rate)
	conc, err := positive(get("QUESTS_EGRESS_CONCURRENCY"), int64(p.Concurrency), "QUESTS_EGRESS_CONCURRENCY")
	if err != nil {
		return Policy{}, err
	}
	p.Concurrency = int(conc)
	if p.Mode == ModeAllowlist && len(p.Allow) == 0 {
		return Policy{}, errors.New("QUESTS_EGRESS_MODE=allowlist needs QUESTS_EGRESS_ALLOW")
	}
	if p.Proxy != nil && p.Mode == ModePublic {
		return Policy{}, errors.New("QUESTS_EGRESS_PROXY: a proxy resolves names where the server cannot check them, so it goes with QUESTS_EGRESS_MODE=allowlist")
	}
	return p, nil
}

func positive(raw string, def int64, name string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s: a positive number (got %q)", name, raw)
	}
	return n, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("an address or a CIDR (got %q)", s)
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// Enabled reports whether the server lets any node go out at all.
func (p Policy) Enabled() bool { return p.Mode == ModePublic || p.Mode == ModeAllowlist }

// HostAllowed says whether the host (already normalised) may be called. In public mode every name passes: the address check at
// the connection is what keeps the internal ones out.
func (p Policy) HostAllowed(host string) bool {
	switch p.Mode {
	case ModePublic:
		return true
	case ModeAllowlist:
		return matchHost(p.Allow, host)
	}
	return false
}

func matchHost(patterns []string, host string) bool {
	for _, pat := range patterns {
		if pat == host {
			return true
		}
		if rest, ok := strings.CutPrefix(pat, "*."); ok && strings.HasSuffix(host, "."+rest) && len(host) > len(rest)+1 {
			return true
		}
	}
	return false
}

// NormalizeHost lower-cases, drops a trailing dot and refuses what is not plain ASCII (a name is punycode here) or an address
// that is not written the way the resolver would read it.
func NormalizeHost(h string) (string, error) {
	h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
	if h == "" {
		return "", errors.New("empty host")
	}
	if a, err := netip.ParseAddr(h); err == nil {
		return a.String(), nil
	}
	if !validHostPattern(h) || strings.HasPrefix(h, "*") {
		return "", fmt.Errorf("bad host %q", h)
	}
	if numericName(h) {
		return "", fmt.Errorf("bad host %q: an address written as a number", h)
	}
	return h, nil
}

// numericName: a name whose last label is a number (`2130706433`, `127.1`, `0x7f.1`): no top-level domain looks like that, and
// some resolvers read such a name as an address.
func numericName(h string) bool {
	last := h[strings.LastIndex(h, ".")+1:]
	if strings.HasPrefix(last, "0x") {
		return true
	}
	for _, r := range last {
		if r < '0' || r > '9' {
			return false
		}
	}
	return last != ""
}

func validHostPattern(h string) bool {
	h = strings.TrimPrefix(h, "*.")
	if h == "" || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
