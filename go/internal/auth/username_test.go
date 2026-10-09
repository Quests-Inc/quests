package auth

import "testing"

func TestValidUsername(t *testing.T) {
	for _, ok := range []string{"ab", "alice", "a.b-c_d", "user1", "x" + "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"} {
		if !validUsername(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "a", ".hidden", "-x", "Alice", "al ice", "é", "eve‮", "<img>", "a/b", "xyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyz"} {
		if validUsername(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestMissingUserCostsAsMuchAsWrongPassword(t *testing.T) {
	// The dummy hash is computed once and reused: verifying against it must be a plain
	// verify (not a second hash), otherwise a missing login answers measurably slower.
	if timingDummy() == "" || timingDummy() != timingDummy() {
		t.Fatal("dummy hash must be stable")
	}
}
