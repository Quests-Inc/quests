package timeutil

import (
	"strings"
	"time"
)

const DefaultDurationSeconds = 24 * 60 * 60

var layouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
}

func NowUTC() time.Time {
	return time.Now().UTC()
}

func EnsureUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	if t.Location() == time.UTC {
		return t
	}
	return t.UTC()
}

// ToDBUTC formats for the database: naive UTC (TIMESTAMP columns hold no zone).
func ToDBUTC(t time.Time) string {
	t = EnsureUTC(t)
	return t.Format("2006-01-02 15:04:05.000000")
}

func ToUTCISO(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	tt := EnsureUTC(*t)
	s := tt.Format("2006-01-02T15:04:05.000000Z")
	return &s
}

func ParseFlexible(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(s, "Z") {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC(), nil
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC(), nil
		}
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, &ParseError{Value: s}
}

type ParseError struct{ Value string }

func (e *ParseError) Error() string { return "invalid datetime: " + e.Value }

func AutoDurationSeconds(deadline, anchor time.Time) int {
	left := int(deadline.Sub(anchor).Seconds())
	if left <= 60 {
		return 60
	}
	if left < DefaultDurationSeconds {
		return left
	}
	return DefaultDurationSeconds
}

func NormalizeDeadline(deadline *time.Time, duration *int, durationExplicit bool, now time.Time) (*time.Time, *int) {
	if deadline == nil {
		return nil, nil
	}
	d := EnsureUTC(*deadline)
	if durationExplicit && duration != nil {
		v := *duration
		if v <= 0 {
			// Explicit 0 (or negative) means "no window": deadline stays as a
			// plain target time, no urgent-window timer, no forced auto-expire.
			zero := 0
			return &d, &zero
		}
		return &d, &v
	}
	sec := AutoDurationSeconds(d, EnsureUTC(now))
	return &d, &sec
}

func RemainingSeconds(deadline *time.Time, now time.Time) *int {
	if deadline == nil {
		return nil
	}
	rem := int(EnsureUTC(*deadline).Sub(EnsureUTC(now)).Seconds())
	return &rem
}

func WindowStart(deadline time.Time, durationSeconds int) time.Time {
	if durationSeconds < 1 {
		durationSeconds = 1
	}
	return EnsureUTC(deadline).Add(-time.Duration(durationSeconds) * time.Second)
}

func IsInUrgentWindow(deadline *time.Time, duration *int, now time.Time) bool {
	if deadline == nil || duration == nil || *duration <= 0 {
		return false
	}
	return WindowStart(*deadline, *duration).Before(EnsureUTC(now))
}

func TimerTone(deadline *time.Time, duration *int, now time.Time) *string {
	if deadline == nil || duration == nil || *duration <= 0 {
		return nil
	}
	rem := RemainingSeconds(deadline, now)
	if rem == nil {
		return nil
	}
	dur := *duration
	if dur < 1 {
		dur = 1
	}
	frac := float64(*rem) / float64(dur)
	var tone string
	switch {
	case frac > 2.0/3.0:
		tone = "green"
	case frac > 1.0/3.0:
		tone = "orange"
	default:
		tone = "red"
	}
	return &tone
}
