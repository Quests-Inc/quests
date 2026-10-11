package identity

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/valirum/quests/go/internal/platform/db"
	"github.com/valirum/quests/go/internal/platform/secretbox"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/tokens"
)

// SessionTTL is how long a browser stays logged in without re-entering the password.
const SessionTTL = 30 * 24 * time.Hour

var (
	ErrNoUser       = errors.New("user not found")
	ErrBadPassword  = errors.New("invalid credentials")
	ErrUserExists   = errors.New("user already exists")
	ErrWeakPassword = errors.New("password must be at least 8 characters")
)

type Store struct {
	DB *sql.DB
	// Box seals the second-factor secrets (nil = two-factor sign-in is not offered).
	Box *secretbox.Box
}

type User struct {
	ID       int64
	Username string
	IsActive bool
	// WorkspaceID is the workspace this session / token works in (see resolveWorkspace).
	WorkspaceID int64
}

// DefaultWorkspaceID is where everything lived before workspaces existed.
const DefaultWorkspaceID = 1

var ErrNotMember = errors.New("not a member of that workspace")

// resolveWorkspace picks the workspace for a session/token: the one it is
// pinned to if the user is still a member, else the user's first membership.
func (s *Store) resolveWorkspace(userID int64, pinned sql.NullInt64, strict bool) (int64, error) {
	if pinned.Valid {
		ok, err := s.IsMember(userID, pinned.Int64)
		if err != nil {
			return 0, err
		}
		if ok {
			return pinned.Int64, nil
		}
		if strict {
			// An API token pinned to a workspace that is gone (or that the user left) must
			// stop working, not quietly start writing into some other workspace.
			return 0, nil
		}
	}
	var id int64
	err := s.DB.QueryRow(`SELECT m.workspace_id FROM workspace_member m JOIN workspace w ON w.id = m.workspace_id
		 WHERE m.user_id = ? AND w.deleted_at IS NULL ORDER BY m.workspace_id LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// No membership (e.g. removed from their last workspace): still logged in,
		// but with no workspace until they create one or accept an invitation.
		return 0, nil
	}
	return id, err
}

func (s *Store) IsMember(userID, workspaceID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT 1 FROM workspace_member m JOIN workspace w ON w.id = m.workspace_id
		 WHERE m.user_id = ? AND m.workspace_id = ? AND w.deleted_at IS NULL`, userID, workspaceID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// SetSessionWorkspace pins a login session to a workspace the user belongs to.
func (s *Store) SetSessionWorkspace(secret string, userID, workspaceID int64) error {
	ok, err := s.IsMember(userID, workspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotMember
	}
	_, err = s.DB.Exec(`UPDATE usersession SET workspace_id = ? WHERE token_hash = ?`, workspaceID, tokens.Hash(secret))
	return err
}

// Principal is whoever made a request: a user, plus how they proved it.
type Principal struct {
	User User
	// Via is "session" for the SPA cookie or "token" for a headless client.
	Via string
	// TokenName is set when Via == "token".
	TokenName string
}

func normUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ErrBadUsername is returned for a login that is not 2–32 plain characters.
var ErrBadUsername = errors.New("логин: 2–32 символа, латиница, цифры, точка, дефис, подчёркивание")

// validUsername keeps logins to a plain, unambiguous alphabet: no look-alike letters,
// no control or direction-override characters, nothing that renders as markup.
func validUsername(u string) bool {
	if len(u) < 2 || len(u) > 32 {
		return false
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '.' || c == '-' || c == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// dummyHash is verified against when the login does not exist, so a missing user costs
// the same as a wrong password. Computed once: hashing per attempt would make the
// missing-user path twice as slow, which is itself an oracle.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

func timingDummy() string {
	dummyHashOnce.Do(func() { dummyHash, _ = HashPassword("quests-timing-equalizer") })
	return dummyHash
}

// CountUsers reports how many accounts exist. Zero means the instance has never
// been bootstrapped, which the server treats as "unconfigured".
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM appuser`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(username, password string) (User, error) {
	username = normUsername(username)
	if !validUsername(username) {
		return User{}, ErrBadUsername
	}
	if len([]rune(password)) < 8 || len(password) > 1024 {
		return User{}, ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	res, err := s.DB.Exec(
		`INSERT INTO appuser (username, password_hash, is_active, created_at) VALUES (?, ?, 1, ?)`,
		username, hash, timeutil.ToDBUTC(timeutil.NowUTC()),
	)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, IsActive: true}, nil
}

func (s *Store) SetPassword(username, password string) error {
	if len([]rune(password)) < 8 {
		return ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE appuser SET password_hash = ? WHERE username = ?`, hash, normUsername(username))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoUser
	}
	// A new password ends every session that was opened with the old one.
	_, err = s.DB.Exec(`DELETE FROM usersession WHERE user_id = (SELECT id FROM appuser WHERE username = ?)`, normUsername(username))
	return err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.DB.Query(`SELECT id, username, is_active FROM appuser ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsActive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Authenticate checks a username/password pair. It always runs a full hash
// comparison, so a missing user costs the same time as a wrong password.
func (s *Store) Authenticate(username, password string) (User, error) {
	var (
		u    User
		hash string
	)
	err := s.DB.QueryRow(
		`SELECT id, username, password_hash, is_active FROM appuser WHERE username = ?`,
		normUsername(username),
	).Scan(&u.ID, &u.Username, &hash, &u.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		// Burn comparable time against a dummy hash to avoid a user-enumeration oracle.
		VerifyPassword(password, timingDummy())
		return User{}, ErrBadPassword
	}
	if err != nil {
		return User{}, err
	}
	if !VerifyPassword(password, hash) {
		return User{}, ErrBadPassword
	}
	if !u.IsActive {
		return User{}, ErrBadPassword
	}
	_, _ = s.DB.Exec(`UPDATE appuser SET last_login_at = ? WHERE id = ?`,
		timeutil.ToDBUTC(timeutil.NowUTC()), u.ID)
	return u, nil
}

// --- sessions ---

func (s *Store) CreateSession(userID int64, userAgent string) (secret string, expires time.Time, err error) {
	secret, hash, err := tokens.New()
	if err != nil {
		return "", time.Time{}, err
	}
	now := timeutil.NowUTC()
	expires = now.Add(SessionTTL)
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	_, err = s.DB.Exec(
		`INSERT INTO usersession (user_id, token_hash, created_at, expires_at, user_agent) VALUES (?, ?, ?, ?, ?)`,
		userID, hash, timeutil.ToDBUTC(now), timeutil.ToDBUTC(expires), userAgent,
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return secret, expires, nil
}

func (s *Store) LookupSession(secret string) (User, error) {
	var (
		u         User
		expiresAt string
		pinned    sql.NullInt64
	)
	err := s.DB.QueryRow(
		`SELECT u.id, u.username, u.is_active, s.expires_at, s.workspace_id
		   FROM usersession s JOIN appuser u ON u.id = s.user_id
		  WHERE s.token_hash = ?`,
		tokens.Hash(secret),
	).Scan(&u.ID, &u.Username, &u.IsActive, &expiresAt, &pinned)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoUser
	}
	if err != nil {
		return User{}, err
	}
	exp, err := timeutil.ParseFlexible(expiresAt)
	if err != nil || !exp.IsZero() && timeutil.NowUTC().After(exp) {
		return User{}, ErrNoUser
	}
	if !u.IsActive {
		return User{}, ErrNoUser
	}
	if u.WorkspaceID, err = s.resolveWorkspace(u.ID, pinned, false); err != nil {
		return User{}, ErrNoUser
	}
	_, _ = s.DB.Exec(`UPDATE usersession SET last_seen_at = ? WHERE token_hash = ?`,
		timeutil.ToDBUTC(timeutil.NowUTC()), tokens.Hash(secret))
	return u, nil
}

func (s *Store) DeleteSession(secret string) error {
	_, err := s.DB.Exec(`DELETE FROM usersession WHERE token_hash = ?`, tokens.Hash(secret))
	return err
}

// PurgeExpiredSessions drops rows past expiry; called opportunistically.
func (s *Store) PurgeExpiredSessions() error {
	_, err := s.DB.Exec(`DELETE FROM usersession WHERE expires_at < ?`, timeutil.ToDBUTC(timeutil.NowUTC()))
	return err
}

// --- API tokens ---

type TokenInfo struct {
	ID         int64
	Name       string
	Username   string
	CreatedAt  string
	LastUsedAt *string
}

// CreateAPIToken returns the secret exactly once; only its hash is persisted.
// workspaceID pins the token to a workspace (0 = the user's first one).
func (s *Store) CreateAPIToken(userID int64, name string, workspaceID int64) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unnamed"
	}
	secret, hash, err := tokens.New()
	if err != nil {
		return "", err
	}
	if workspaceID > 0 {
		ok, err := s.IsMember(userID, workspaceID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrNotMember
		}
		_, err = s.DB.Exec(
			`INSERT INTO apitoken (user_id, name, token_hash, created_at, workspace_id) VALUES (?, ?, ?, ?, ?)`,
			userID, name, hash, timeutil.ToDBUTC(timeutil.NowUTC()), workspaceID,
		)
		if err != nil {
			return "", err
		}
		return secret, nil

	}
	_, err = s.DB.Exec(
		`INSERT INTO apitoken (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, hash, timeutil.ToDBUTC(timeutil.NowUTC()),
	)
	if err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) LookupAPIToken(secret string) (User, string, error) {
	var (
		u      User
		name   string
		pinned sql.NullInt64
	)
	hash := tokens.Hash(secret)
	err := s.DB.QueryRow(
		`SELECT u.id, u.username, u.is_active, t.name, t.workspace_id
		   FROM apitoken t JOIN appuser u ON u.id = t.user_id
		  WHERE t.token_hash = ? AND t.revoked_at IS NULL`,
		hash,
	).Scan(&u.ID, &u.Username, &u.IsActive, &name, &pinned)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNoUser
	}
	if err != nil {
		return User{}, "", err
	}
	if !u.IsActive {
		return User{}, "", ErrNoUser
	}
	if u.WorkspaceID, err = s.resolveWorkspace(u.ID, pinned, true); err != nil {
		return User{}, "", ErrNoUser
	}
	_, _ = s.DB.Exec(`UPDATE apitoken SET last_used_at = ? WHERE token_hash = ?`,
		timeutil.ToDBUTC(timeutil.NowUTC()), hash)
	return u, name, nil
}

func (s *Store) ListAPITokens() ([]TokenInfo, error) {
	rows, err := s.DB.Query(
		`SELECT t.id, t.name, u.username, t.created_at, t.last_used_at
		   FROM apitoken t JOIN appuser u ON u.id = t.user_id
		  WHERE t.revoked_at IS NULL ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TokenInfo
	for rows.Next() {
		var t TokenInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.Username, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(id int64) error {
	res, err := s.DB.Exec(`UPDATE apitoken SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		timeutil.ToDBUTC(timeutil.NowUTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no such active token")
	}
	return nil
}

// DeleteUser removes an account (used to undo a signup whose invitation was lost to a race).
func (s *Store) DeleteUser(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM appuser WHERE id = ?`, id)
	return err
}

// --- a user's own tokens (self-service from the web UI) ---

// MaxUserTokens bounds how many live tokens one account may hold.
const MaxUserTokens = 25

type UserToken struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

// ListUserTokens returns the user's live tokens that work in the workspace. A token
// not pinned to any workspace works in the user's first one.
func (s *Store) ListUserTokens(userID, workspaceID int64) ([]UserToken, error) {
	q := `SELECT id, name, created_at, last_used_at FROM apitoken WHERE user_id = ? AND revoked_at IS NULL`
	args := []any{userID}
	def, err := s.resolveWorkspace(userID, sql.NullInt64{}, false)
	if err != nil {
		return nil, err
	}
	// Parameters of an untyped "? = ?" cannot be inferred by PostgreSQL: decide in Go.
	if workspaceID == def {
		q += ` AND (workspace_id = ? OR workspace_id IS NULL)`
	} else {
		q += ` AND workspace_id = ?`
	}
	args = append(args, workspaceID)

	rows, err := s.DB.Query(q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserToken{}
	for rows.Next() {
		var t UserToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountUserTokens counts the user's live tokens in every workspace.
func (s *Store) CountUserTokens(userID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM apitoken WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&n)
	return n, err
}

// RevokeUserToken revokes one of the user's own tokens (never someone else's).
func (s *Store) RevokeUserToken(userID, tokenID int64) error {
	res, err := s.DB.Exec(`UPDATE apitoken SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		timeutil.ToDBUTC(timeutil.NowUTC()), tokenID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no such active token")
	}
	return nil
}

// DeleteOtherSessions ends every session of the account except the one holding keepSecret
// (a fresh second factor should not leave an older, possibly stolen, session logged in).
func (s *Store) DeleteOtherSessions(userID int64, keepSecret string) error {
	_, err := s.DB.Exec(`DELETE FROM usersession WHERE user_id = ? AND token_hash <> ?`, userID, tokens.Hash(keepSecret))
	return err
}
