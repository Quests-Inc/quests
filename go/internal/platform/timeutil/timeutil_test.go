package timeutil_test

import (
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

func TestAutoDurationCapsAt24h(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(10 * 24 * time.Hour)
	if got := timeutil.AutoDurationSeconds(deadline, now); got != timeutil.DefaultDurationSeconds {
		t.Fatalf("got %d want %d", got, timeutil.DefaultDurationSeconds)
	}
}

func TestAutoDurationShort(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(3 * time.Hour)
	if got := timeutil.AutoDurationSeconds(deadline, now); got != 3*3600 {
		t.Fatalf("got %d", got)
	}
}

func TestNormalizeClearsWithoutDeadline(t *testing.T) {
	dur := 3600
	d, out := timeutil.NormalizeDeadline(nil, &dur, true, time.Now().UTC())
	if d != nil || out != nil {
		t.Fatalf("want nils got %v %v", d, out)
	}
}
