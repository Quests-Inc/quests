package ratelimit

import "testing"

func TestUserPenaltyDoesNotLockOut(t *testing.T) {
	l := New()
	for i := 0; i < userFreeFails; i++ {
		l.RecordUserFailure("owner")
	}
	if d := l.UserPenalty("owner"); d != 0 {
		t.Fatalf("no stall expected within the free failures, got %v", d)
	}
	for i := 0; i < 20; i++ {
		l.RecordUserFailure("owner")
	}
	if d := l.UserPenalty("owner"); d <= 0 || d > userMaxPenalty {
		t.Fatalf("penalty %v should be a bounded stall", d)
	}
}
