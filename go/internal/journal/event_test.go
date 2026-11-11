package journal

import (
	"testing"
	"time"
)

func TestResolveEventDeadline(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
	got, err := ResolveEventDeadline(start, end, EventEdgeStart, EventDirBefore, 3*86400)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("before start: got %s", got)
	}
	got, err = ResolveEventDeadline(start, end, EventEdgeEnd, EventDirAfter, 2*3600)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2027, 1, 2, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("after end: got %s", got)
	}
	if _, err := ResolveEventDeadline(start, end, "window", EventDirBefore, 0); err == nil {
		t.Fatal("expected a bad edge")
	}
}
