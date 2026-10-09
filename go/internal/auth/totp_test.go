package auth

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B: secret "12345678901234567890", SHA-1; 8-digit vectors truncated to the
// last six digits (this implementation is 6-digit).
func TestTOTPRFCVectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		unix int64
		want string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		got, err := totpCode(secret, c.unix/30)
		if err != nil || got != c.want {
			t.Errorf("t=%d: got %q (%v), want %q", c.unix, got, err, c.want)
		}
	}
}

func TestMatchTOTPWindowAndRejects(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	cur := StepAt(now)
	good, _ := totpCode(secret, cur)
	if step, ok := MatchTOTP(secret, good, now); !ok || step != cur {
		t.Fatalf("current code refused: %v %v", step, ok)
	}
	prev, _ := totpCode(secret, cur-1)
	next, _ := totpCode(secret, cur+1)
	if _, ok := MatchTOTP(secret, prev, now); !ok {
		t.Error("previous step must be accepted (clock drift)")
	}
	if _, ok := MatchTOTP(secret, next, now); !ok {
		t.Error("next step must be accepted (clock drift)")
	}
	far, _ := totpCode(secret, cur+5)
	if _, ok := MatchTOTP(secret, far, now); ok {
		t.Error("a code five steps away must be refused")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := MatchTOTP(secret, bad, now); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if step, ok := MatchTOTP(secret, good[:3]+" "+good[3:], now); !ok || step != cur {
		t.Error("a code typed with a space must work")
	}
}

func TestTOTPURI(t *testing.T) {
	u := TOTPURI("Quests", "alice", "JBSWY3DPEHPK3PXP")
	for _, want := range []string{"otpauth://totp/Quests:alice?", "secret=JBSWY3DPEHPK3PXP", "issuer=Quests", "digits=6", "period=30"} {
		if !strings.Contains(u, want) {
			t.Errorf("%q missing from %q", want, u)
		}
	}
}
