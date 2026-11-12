package model

import (
	"testing"
)

func TestFormatRemaining(t *testing.T) {
	if got := FormatRemaining(65); got != "1:05" {
		t.Fatalf("got %q", got)
	}
	if got := FormatRemaining(3661); got != "1:01:01" {
		t.Fatalf("got %q", got)
	}
}
