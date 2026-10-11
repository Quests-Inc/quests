package schedule

import (
	"strconv"
	"strings"
	"time"
)

func templateDueToday(freq, weekdays string, localNow time.Time) bool {
	if strings.ToLower(freq) == "daily" || freq == "" {
		return true
	}
	days := parseWeekdays(weekdays)
	if len(days) == 0 {
		return true
	}
	// Python weekday(): Mon=0 … Sun=6. Go: Sun=0 … Sat=6.
	py := (int(localNow.Weekday()) + 6) % 7
	_, ok := days[py]
	return ok
}

// TemplateDueTodayForTest exports weekday matching for unit tests.
func TemplateDueTodayForTest(freq, weekdays string, localNow time.Time) bool {
	return templateDueToday(freq, weekdays, localNow)
}

func parseWeekdays(raw string) map[int]struct{} {
	out := map[int]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 6 {
			continue
		}
		out[n] = struct{}{}
	}
	return out
}

func parseClock(raw string) (hour, min, sec int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, false
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	s := 0
	if len(parts) > 2 {
		s, _ = strconv.Atoi(parts[2])
	}
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 || s < 0 || s > 59 {
		return 0, 0, 0, false
	}
	return h, m, s, true
}

func fixedDeadline(tmpl templateRow, localNow time.Time, loc *time.Location) (*time.Time, *int) {
	if !tmpl.DeadlineTime.Valid {
		return nil, nil
	}
	h, m, s, ok := parseClock(tmpl.DeadlineTime.String)
	if !ok {
		return nil, nil
	}
	deadlineLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), h, m, s, 0, loc)
	deadlineUTC := deadlineLocal.UTC()
	var duration int
	if tmpl.DurationSeconds.Valid {
		duration = int(tmpl.DurationSeconds.Int64)
		if duration < 1 {
			duration = 1
		}
	} else {
		startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
		if !deadlineLocal.After(startLocal) {
			startLocal = localNow
		}
		duration = int(deadlineLocal.Sub(startLocal).Seconds())
		if duration < 60 {
			duration = 60
		}
	}
	return &deadlineUTC, &duration
}

// emitPoolOpenAt is the moment a fixed-mode pool template's own check window
// opens: duration_seconds before its deadline. Before this moment,
// MaterializeDue skips the template entirely for the tick instead of running
// emit_pool_command — see the call site's comment for why.
func emitPoolOpenAt(deadline time.Time, durationSeconds int) time.Time {
	return deadline.Add(-time.Duration(durationSeconds) * time.Second)
}

func surpriseDeadline(tmpl templateRow, nowUTC time.Time) (*time.Time, *int) {
	if !tmpl.DurationSeconds.Valid {
		return nil, nil
	}
	duration := int(tmpl.DurationSeconds.Int64)
	if duration < 1 {
		duration = 1
	}
	deadline := nowUTC.Add(time.Duration(duration) * time.Second)
	return &deadline, &duration
}
