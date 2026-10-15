package model

import (
	"testing"
	"time"
)

func TestFormatRemaining(t *testing.T) {
	if got := FormatRemaining(65); got != "1:05" {
		t.Fatalf("got %q", got)
	}
	if got := FormatRemaining(3661); got != "1:01:01" {
		t.Fatalf("got %q", got)
	}
}

func TestIsUrgentWindow(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 1, 0, time.UTC) // just inside window
	deadline := "2026-10-14T12:10:00Z"
	dur := 600 // 10 min window starting 12:00
	if !IsUrgent(&deadline, &dur, now) {
		t.Fatal("expected urgent inside window")
	}
	early := time.Date(2026, 10, 14, 11, 59, 0, 0, time.UTC)
	if IsUrgent(&deadline, &dur, early) {
		t.Fatal("not urgent before window")
	}
}
