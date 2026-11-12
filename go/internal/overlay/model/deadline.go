package model

import (
	"fmt"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
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

func deadlineState(q api.Quest, now time.Time) timeutil.DeadlineState {
	d, ok := parseUTC(q.DeadlineAt)
	if !ok {
		return timeutil.DeadlineState{}
	}
	expire := q.ExpireOnDeadline == nil || *q.ExpireOnDeadline
	return timeutil.DeadlineAt(&d, q.DurationSeconds, expire, now)
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
