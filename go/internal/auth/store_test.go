package auth

import (
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/platform/pgtest"
)

// newTestStore is a Store on a fresh PostgreSQL database with the real schema, set up
// as production runs it (workspaces on).
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{DB: pgtest.New(t)}
}

func TestCreateUser(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("Alice", "correcthorse")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("username = %q, want lowercased %q", u.Username, "alice")
	}
	if u.ID == 0 {
		t.Error("expected a non-zero id")
	}
	if !u.IsActive {
		t.Error("expected a new user to be active")
	}

	if _, err := s.CreateUser("alice", "anotherpassword"); err != ErrUserExists {
		t.Errorf("duplicate username: err = %v, want ErrUserExists", err)
	}

	if _, err := s.CreateUser("bob", "short"); err != ErrWeakPassword {
		t.Errorf("short password: err = %v, want ErrWeakPassword", err)
	}

	if _, err := s.CreateUser("   ", "correcthorse"); err == nil {
		t.Error("expected an error for a blank username")
	}
}

func TestAuthenticate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, err := s.Authenticate("alice", "correcthorse"); err != nil {
		t.Errorf("correct password: err = %v, want nil", err)
	}

	// Case-insensitive username, same as CreateUser's normalization.
	if _, err := s.Authenticate("ALICE", "correcthorse"); err != nil {
		t.Errorf("uppercased username: err = %v, want nil", err)
	}

	if _, err := s.Authenticate("alice", "wrongpassword"); err != ErrBadPassword {
		t.Errorf("wrong password: err = %v, want ErrBadPassword", err)
	}

	if _, err := s.Authenticate("nobody", "whatever"); err != ErrBadPassword {
		t.Errorf("unknown user: err = %v, want ErrBadPassword (not ErrNoUser — no user-enumeration oracle)", err)
	}
}

// newOwnerStore is newTestStore on the owner role: the host's admin commands change
// passwords and account state, which the API role may not.
func newOwnerStore(t *testing.T) *Store {
	t.Helper()
	_, url := pgtest.NewWithURL(t)
	return &Store{DB: pgtest.Owner(t, url)}
}

func TestAuthenticateInactiveUser(t *testing.T) {
	s := newOwnerStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := s.DB.Exec(`UPDATE appuser SET is_active = 0 WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := s.Authenticate("alice", "correcthorse"); err != ErrBadPassword {
		t.Errorf("deactivated user: err = %v, want ErrBadPassword", err)
	}
}

func TestSetPassword(t *testing.T) {
	s := newOwnerStore(t)
	if _, err := s.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.SetPassword("alice", "newpassword1"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, err := s.Authenticate("alice", "correcthorse"); err != ErrBadPassword {
		t.Error("old password should no longer work")
	}
	if _, err := s.Authenticate("alice", "newpassword1"); err != nil {
		t.Errorf("new password: err = %v, want nil", err)
	}

	if err := s.SetPassword("alice", "short"); err != ErrWeakPassword {
		t.Errorf("short new password: err = %v, want ErrWeakPassword", err)
	}

	if err := s.SetPassword("nobody", "somepassword"); err != ErrNoUser {
		t.Errorf("unknown user: err = %v, want ErrNoUser", err)
	}
}

func TestListUsersAndCountUsers(t *testing.T) {
	s := newTestStore(t)
	if n, err := s.CountUsers(); err != nil || n != 0 {
		t.Fatalf("CountUsers on empty store = (%d, %v), want (0, nil)", n, err)
	}

	if _, err := s.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "correcthorse"); err != nil {
		t.Fatal(err)
	}

	n, err := s.CountUsers()
	if err != nil || n != 2 {
		t.Fatalf("CountUsers = (%d, %v), want (2, nil)", n, err)
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 || users[0].Username != "alice" || users[1].Username != "bob" {
		t.Errorf("ListUsers = %+v, want [alice bob] in id order", users)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}

	secret, expires, err := s.CreateSession(u.ID, "test-agent")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if secret == "" {
		t.Fatal("expected a non-empty session secret")
	}
	if !expires.After(time.Now()) {
		t.Errorf("expires = %v, want a future time", expires)
	}

	got, err := s.LookupSession(secret)
	if err != nil {
		t.Fatalf("LookupSession: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("LookupSession user id = %d, want %d", got.ID, u.ID)
	}

	if _, err := s.LookupSession("not-a-real-secret"); err != ErrNoUser {
		t.Errorf("bogus secret: err = %v, want ErrNoUser", err)
	}

	if err := s.DeleteSession(secret); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.LookupSession(secret); err != ErrNoUser {
		t.Errorf("after DeleteSession: err = %v, want ErrNoUser", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateSession(u.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	// Back-date the session past its expiry directly — CreateSession always
	// mints a live one, so this is the only way to exercise the expiry path.
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02 15:04:05.000000")
	if _, err := s.DB.Exec(`UPDATE usersession SET expires_at = ? WHERE token_hash = ?`, past, HashToken(secret)); err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	if _, err := s.LookupSession(secret); err != ErrNoUser {
		t.Errorf("expired session: err = %v, want ErrNoUser", err)
	}
}

func TestPurgeExpiredSessions(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	liveSecret, _, err := s.CreateSession(u.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	expiredSecret, _, err := s.CreateSession(u.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02 15:04:05.000000")
	if _, err := s.DB.Exec(`UPDATE usersession SET expires_at = ? WHERE token_hash = ?`, past, HashToken(expiredSecret)); err != nil {
		t.Fatal(err)
	}

	if err := s.PurgeExpiredSessions(); err != nil {
		t.Fatalf("PurgeExpiredSessions: %v", err)
	}

	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM usersession`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("usersession rows after purge = %d, want 1 (only the live one)", count)
	}
	if _, err := s.LookupSession(liveSecret); err != nil {
		t.Errorf("live session should survive the purge: err = %v", err)
	}
}

func TestAPITokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}

	secret, err := s.CreateAPIToken(u.ID, "overlay", 0)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if secret == "" {
		t.Fatal("expected a non-empty token secret")
	}

	gotUser, gotName, err := s.LookupAPIToken(secret)
	if err != nil {
		t.Fatalf("LookupAPIToken: %v", err)
	}
	if gotUser.ID != u.ID {
		t.Errorf("LookupAPIToken user id = %d, want %d", gotUser.ID, u.ID)
	}
	if gotName != "overlay" {
		t.Errorf("LookupAPIToken name = %q, want %q", gotName, "overlay")
	}

	if _, _, err := s.LookupAPIToken("not-a-real-secret"); err != ErrNoUser {
		t.Errorf("bogus secret: err = %v, want ErrNoUser", err)
	}

	tokens, err := s.ListAPITokens()
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "overlay" {
		t.Errorf("ListAPITokens = %+v, want one token named overlay", tokens)
	}

	if err := s.RevokeAPIToken(tokens[0].ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if _, _, err := s.LookupAPIToken(secret); err != ErrNoUser {
		t.Errorf("revoked token: err = %v, want ErrNoUser", err)
	}
	if err := s.RevokeAPIToken(tokens[0].ID); err == nil {
		t.Error("revoking an already-revoked token should error")
	}

	remaining, err := s.ListAPITokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Errorf("ListAPITokens after revoke = %+v, want none (revoked tokens are excluded)", remaining)
	}
}

func TestCreateAPITokenDefaultsName(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.CreateAPIToken(u.ID, "   ", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, name, err := s.LookupAPIToken(secret)
	if err != nil {
		t.Fatal(err)
	}
	if name != "unnamed" {
		t.Errorf("blank name = %q, want %q", name, "unnamed")
	}
}
