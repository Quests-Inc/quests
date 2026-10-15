package model

import (
	"fmt"
	"strings"
	"time"
)

func parseUTC(value *string) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return time.Time{}, false
	}
	if strings.HasSuffix(text, "Z") {
		text = text[:len(text)-1] + "+00:00"
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		t, err = time.Parse(time.RFC3339, text)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// RemainingSeconds until deadline (UTC ISO from API).
func RemainingSeconds(deadlineAt *string, now time.Time) (int, bool) {
	deadline, ok := parseUTC(deadlineAt)
	if !ok {
		return 0, false
	}
	return int(deadline.Sub(now.UTC()).Seconds()), true
}

// IsUrgent is true in [deadline−duration, deadline] or after deadline.
func IsUrgent(deadlineAt *string, durationSeconds *int, now time.Time) bool {
	if rem, ok := RemainingSeconds(deadlineAt, now); ok && rem <= 0 {
		return true
	}
	deadline, ok := parseUTC(deadlineAt)
	if !ok || durationSeconds == nil || *durationSeconds <= 0 {
		return false
	}
	start := deadline.Add(-time.Duration(*durationSeconds) * time.Second)
	// Match Python: start < now (exclusive of the exact start instant).
	return now.UTC().After(start)
}

// TimerTone is green|orange|red by remaining fraction of duration.
func TimerTone(deadlineAt *string, durationSeconds *int, now time.Time) string {
	rem, ok := RemainingSeconds(deadlineAt, now)
	if !ok || durationSeconds == nil || *durationSeconds <= 0 {
		return ""
	}
	frac := float64(rem) / float64(*durationSeconds)
	switch {
	case frac > 2.0/3.0:
		return "green"
	case frac > 1.0/3.0:
		return "orange"
	default:
		return "red"
	}
}

// FormatRemaining renders a countdown like m:ss or h:mm:ss.
func FormatRemaining(seconds int) string {
	sign := ""
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, s)
	}
	return fmt.Sprintf("%s%d:%02d", sign, m, s)
}
