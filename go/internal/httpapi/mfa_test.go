package httpapi

import (
	"testing"
	"time"
)

func TestMFATicketIsAttemptLimitedAndSingleUse(t *testing.T) {
	var tk mfaTickets
	id, err := tk.issue(7, "alice")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < mfaTicketAttempts; i++ {
		if got, ok := tk.attempt(id); !ok || got.userID != 7 {
			t.Fatalf("attempt %d refused: %v %v", i+1, got, ok)
		}
	}
	if _, ok := tk.attempt(id); ok {
		t.Fatal("a ticket must die after its attempts are used up")
	}
	if _, ok := tk.attempt(id); ok {
		t.Fatal("a dead ticket came back")
	}

	id2, _ := tk.issue(8, "bob")
	if _, ok := tk.attempt(id2); !ok {
		t.Fatal("fresh ticket refused")
	}
	tk.spend(id2)
	if _, ok := tk.attempt(id2); ok {
		t.Fatal("a spent ticket must not work again")
	}
	if _, ok := tk.attempt("made-up"); ok {
		t.Fatal("an unknown ticket was accepted")
	}
}

func TestMFATicketExpires(t *testing.T) {
	var tk mfaTickets
	id, _ := tk.issue(1, "a")
	tk.mu.Lock()
	tk.m[id].expires = time.Now().Add(-time.Second)
	tk.mu.Unlock()
	if _, ok := tk.attempt(id); ok {
		t.Fatal("an expired ticket was accepted")
	}
}
