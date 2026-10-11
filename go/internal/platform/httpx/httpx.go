// Package httpx holds the small request/response helpers every HTTP-facing package needs.
package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// WriteJSON answers with v as JSON (no HTML escaping: the web client renders text, not markup).
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// WriteErr answers with {"detail": detail}.
func WriteErr(w http.ResponseWriter, status int, detail string) {
	WriteJSON(w, status, map[string]string{"detail": detail})
}

var sourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,31}$`)

// RequestSource is how a thing was made, from the X-Quests-Source header the clients set
// ("web", "cli", "mcp", "telegram"…); "api" when absent or not a plain label.
func RequestSource(r *http.Request) *string {
	v := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Quests-Source")))
	if !sourcePattern.MatchString(v) {
		v = "api"
	}
	return &v
}

// BearerToken is the token of an "Authorization: Bearer …" header ("" when absent).
func BearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// IsHTTPS reports whether this request arrived over TLS, directly or through a reverse proxy that sets
// X-Forwarded-Proto. Trusting the header is safe under this deployment: the only ways in are the LAN directly
// or the Tailscale-only gateway, nothing untrusted can reach the port to spoof it.
func IsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// ClientKey identifies the caller for throttling. X-Forwarded-For is
// trusted here for the same reason IsHTTPS trusts X-Forwarded-Proto:
// under this deployment the only paths in are the LAN directly or the
// Tailscale-only edge gateway, nothing untrusted can reach this port to
// spoof the header. Falls back to RemoteAddr when absent (direct LAN calls).
func ClientKey(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip.String()
		}
	}
	// RemoteAddr carries the ephemeral port: keyed on it, every new connection
	// would be a new client and the throttle would never engage.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
