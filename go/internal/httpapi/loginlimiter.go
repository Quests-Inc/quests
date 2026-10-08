package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginLimiter throttles repeated failed logins per source IP with
// exponential backoff: 5 free attempts, then a lockout that doubles (30s,
// 1m, 2m, ... capped at 15m) on every further failure while still locked.
// Process-memory only — resets on restart, which is fine for a personal
// single-instance server; it only needs to blunt an automated password
// guesser, not survive a distributed attack.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempts
	invites  map[string][]time.Time
}

type loginAttempts struct {
	fails       int
	lockedUntil time.Time
}

const (
	loginFreeAttempts = 5
	loginBaseLockout  = 30 * time.Second
	loginMaxLockout   = 15 * time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempts), invites: make(map[string][]time.Time)}
}

const (
	inviteWindow = 10 * time.Minute
	inviteMax    = 30
)

// tryInvite counts one invitation-link request from key and reports whether it
// is within budget (inviteMax per inviteWindow). Tokens are 256-bit, so this
// is hygiene against enumeration noise, not the actual protection.
func (l *loginLimiter) tryInvite(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.invites[key][:0]
	for _, t := range l.invites[key] {
		if now.Sub(t) < inviteWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= inviteMax {
		l.invites[key] = recent
		return false
	}
	l.invites[key] = append(recent, now)
	return true
}

// allow reports whether a login attempt from key may proceed, and if not,
// how long until it may retry.
func (l *loginLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return true, 0
	}
	if a.lockedUntil.IsZero() || time.Now().After(a.lockedUntil) {
		return true, 0
	}
	return false, time.Until(a.lockedUntil)
}

func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		a = &loginAttempts{}
		l.attempts[key] = a
	}
	a.fails++
	if a.fails <= loginFreeAttempts {
		return
	}
	lockout := loginBaseLockout << uint(a.fails-loginFreeAttempts-1)
	if lockout <= 0 || lockout > loginMaxLockout {
		lockout = loginMaxLockout
	}
	a.lockedUntil = time.Now().Add(lockout)
}

func (l *loginLimiter) recordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// loginClientKey identifies the caller for throttling. X-Forwarded-For is
// trusted here for the same reason requestIsHTTPS trusts X-Forwarded-Proto:
// under this deployment the only paths in are the LAN directly or the
// Tailscale-only edge gateway, nothing untrusted can reach this port to
// spoof the header. Falls back to RemoteAddr when absent (direct LAN calls).
func loginClientKey(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	return r.RemoteAddr
}
