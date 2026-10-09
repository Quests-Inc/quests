package httpapi

import (
	"net"
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
	// byUser counts recent failures per login name, from anywhere: the per-IP throttle
	// does not stop a guesser who rotates addresses (or a header).
	byUser map[string][]time.Time
	// slots bounds concurrent password hashing: every attempt costs ~100ms of CPU.
	slots chan struct{}
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
	return &loginLimiter{
		attempts: make(map[string]*loginAttempts),
		invites:  make(map[string][]time.Time),
		byUser:   make(map[string][]time.Time),
		slots:    make(chan struct{}, 4),
	}
}

const (
	userWindow     = 15 * time.Minute
	userFreeFails  = 10
	userMaxPenalty = 5 * time.Second
)

// acquire takes a hashing slot; false means the server is busy hashing already.
func (l *loginLimiter) acquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *loginLimiter) release() { <-l.slots }

// userPenalty is how long to stall an attempt on this login after many recent
// failures. A stall, not a lockout: whoever guesses cannot lock the owner out.
func (l *loginLimiter) userPenalty(user string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.byUser[user][:0]
	for _, t := range l.byUser[user] {
		if now.Sub(t) < userWindow {
			recent = append(recent, t)
		}
	}
	l.byUser[user] = recent
	if len(recent) <= userFreeFails {
		return 0
	}
	d := 250 * time.Millisecond << uint(min(len(recent)-userFreeFails, 5))
	return min(d, userMaxPenalty)
}

func (l *loginLimiter) recordUserFailure(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.byUser) > 10000 { // names are attacker-chosen: keep the map bounded
		l.byUser = make(map[string][]time.Time)
	}
	l.byUser[user] = append(l.byUser[user], time.Now())
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
