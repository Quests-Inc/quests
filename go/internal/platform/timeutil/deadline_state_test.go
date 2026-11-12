package timeutil_test

import (
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"testing"
	"time"
)

func TestDeadlineStateBoundaries(t *testing.T) {
	end := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	window := 3600
	zero := 0
	for _, tc := range []struct {
		name            string
		at              time.Time
		window          *int
		expire          bool
		tone            string
		urgent, reached bool
	}{
		{"outside", end.Add(-2 * time.Hour), &window, true, "green", false, false},
		{"opening inclusive", end.Add(-time.Hour), &window, true, "green", true, false},
		{"two thirds", end.Add(-40 * time.Minute), &window, true, "orange", true, false},
		{"one third", end.Add(-20 * time.Minute), &window, true, "red", true, false},
		{"neutral", end.Add(-time.Minute), &zero, true, "", false, false},
		{"hard due", end, &zero, true, "overdue", true, true},
		{"soft due", end, &window, false, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := timeutil.DeadlineAt(&end, tc.window, tc.expire, tc.at)
			tone := ""
			if s.Tone != nil {
				tone = *s.Tone
			}
			if tone != tc.tone || s.Urgent != tc.urgent || s.Reached != tc.reached {
				t.Fatalf("got %+v tone=%q", s, tone)
			}
		})
	}
}
