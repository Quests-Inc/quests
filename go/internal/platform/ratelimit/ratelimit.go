// Package ratelimit throttles abuse that is cheap to try and costly to serve: password guessing, invitation
// probing, paid assistant calls. In-memory only; a restart resets it, which is fine for a single instance.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter throttles repeated failed logins per source IP with
// exponential backoff: 5 free attempts, then a lockout that doubles (30s,
// 1m, 2m, ... capped at 15m) on every further failure while still locked.
// Process-memory only — resets on restart, which is fine for a personal
// single-instance server; it only needs to blunt an automated password
// guesser, not survive a distributed attack.
type Limiter struct {
	mu       sync.Mutex
	attempts map[string]*attempts
	invites  map[string][]time.Time
	// byUser counts recent failures per login name, from anywhere: the per-IP throttle
	// does not stop a guesser who rotates addresses (or a header).
	byUser map[string][]time.Time
	// llm counts assistant requests per account (each one is a paid call somewhere).
	llm map[string][]time.Time
	// slots bounds concurrent password hashing: every attempt costs ~100ms of CPU.
	slots chan struct{}
}

type attempts struct {
	fails       int
	lockedUntil time.Time
}

const (
	// FreeAttempts failed logins are allowed before the lockout starts.
	FreeAttempts     = 5
	loginBaseLockout = 30 * time.Second
	loginMaxLockout  = 15 * time.Minute
)

// New returns an empty limiter.
func New() *Limiter {
	return &Limiter{
		attempts: make(map[string]*attempts),
		invites:  make(map[string][]time.Time),
		byUser:   make(map[string][]time.Time),
		llm:      make(map[string][]time.Time),
		slots:    make(chan struct{}, 4),
	}
}

const (
	userWindow     = 15 * time.Minute
	userFreeFails  = 10
	userMaxPenalty = 5 * time.Second
)

const (
	llmWindow = 10 * time.Minute
	llmMax    = 20
)

// TryLLM counts one assistant request from key and reports whether it is within budget.
func (l *Limiter) TryLLM(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.llm[key][:0]
	for _, t := range l.llm[key] {
		if now.Sub(t) < llmWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= llmMax {
		l.llm[key] = recent
		return false
	}
	l.llm[key] = append(recent, now)
	return true
}

// Acquire takes a hashing slot; false means the server is busy hashing already.
func (l *Limiter) Acquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *Limiter) Release() { <-l.slots }

// UserPenalty is how long to stall an attempt on this login after many recent
// failures. A stall, not a lockout: whoever guesses cannot lock the owner out.
func (l *Limiter) UserPenalty(user string) time.Duration {
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

func (l *Limiter) RecordUserFailure(user string) {
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

// TryInvite counts one invitation-link request from key and reports whether it
// is within budget (inviteMax per inviteWindow). Tokens are 256-bit, so this
// is hygiene against enumeration noise, not the actual protection.
func (l *Limiter) TryInvite(key string) bool {
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

// Allow reports whether a login attempt from key may proceed, and if not,
// how long until it may retry.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
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

func (l *Limiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		a = &attempts{}
		l.attempts[key] = a
	}
	a.fails++
	if a.fails <= FreeAttempts {
		return
	}
	lockout := loginBaseLockout << uint(a.fails-FreeAttempts-1)
	if lockout <= 0 || lockout > loginMaxLockout {
		lockout = loginMaxLockout
	}
	a.lockedUntil = time.Now().Add(lockout)
}

func (l *Limiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
