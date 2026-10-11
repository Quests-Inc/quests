package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
)

func newBox(t *testing.T) *secretbox.Box {
	t.Helper()
	t.Setenv("QUESTS_SECRET_KEY", "")
	b, err := secretbox.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// With the server key lost or replaced the sealed TOTP secret cannot be opened. A recovery code,
// which does not depend on it, must still get the person in (once); a plain code must not.
func TestRecoveryCodeWorksWhenTheKeyIsGone(t *testing.T) {
	st := &Store{DB: pgtest.New(t), Box: newBox(t)}
	u, err := st.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	uid := u.ID

	secret, err := st.BeginMFA(uid)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totpCode(secret, StepAt(time.Now()))
	codes, err := st.ConfirmMFA(uid, code)
	if err != nil || len(codes) != 10 {
		t.Fatalf("enrol: %v %v", len(codes), err)
	}

	st.Box = newBox(t) // another key: the sealed secret no longer opens
	next, _ := totpCode(secret, StepAt(time.Now())+1)
	if err := st.VerifySecondFactor(uid, next); err == nil {
		t.Fatal("a TOTP code was accepted although its secret cannot be opened")
	}
	if err := st.VerifySecondFactor(uid, codes[0]); err != nil {
		t.Fatalf("a recovery code must still work without the key: %v", err)
	}
	if err := st.VerifySecondFactor(uid, codes[0]); !errors.Is(err, ErrBadCode) {
		t.Fatalf("a recovery code worked twice: %v", err)
	}
}
