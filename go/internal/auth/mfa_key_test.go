package auth

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valirum/quests/go/internal/secretbox"
)

const mfaTestSchema = `
CREATE TABLE user_totp (user_id INTEGER PRIMARY KEY, secret_enc TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, last_step INTEGER NOT NULL DEFAULT 0, created_at TEXT);
CREATE TABLE user_recovery_code (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, code_hash TEXT NOT NULL, used_at TEXT, created_at TEXT);
`

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
	db, err := sql.Open("sqlite", "file:mfakey?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(mfaTestSchema); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: db, Workspaces: true, Box: newBox(t)}

	secret, err := st.BeginMFA(1)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totpCode(secret, StepAt(time.Now()))
	codes, err := st.ConfirmMFA(1, code)
	if err != nil || len(codes) != 10 {
		t.Fatalf("enrol: %v %v", len(codes), err)
	}

	st.Box = newBox(t) // another key: the sealed secret no longer opens
	next, _ := totpCode(secret, StepAt(time.Now())+1)
	if err := st.VerifySecondFactor(1, next); err == nil {
		t.Fatal("a TOTP code was accepted although its secret cannot be opened")
	}
	if err := st.VerifySecondFactor(1, codes[0]); err != nil {
		t.Fatalf("a recovery code must still work without the key: %v", err)
	}
	if err := st.VerifySecondFactor(1, codes[0]); !errors.Is(err, ErrBadCode) {
		t.Fatalf("a recovery code worked twice: %v", err)
	}
}
