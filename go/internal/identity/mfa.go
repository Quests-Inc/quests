package identity

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/tokens"
)

// Two-factor sign-in (quest=359). The secret is sealed with the server key; recovery codes
// are stored as hashes and spend once. Accounts without a row sign in with the password alone.

var (
	ErrMFAUnavailable = errors.New("двухфакторный вход недоступен на этом сервере (нужен PostgreSQL)")
	ErrMFAEnabled     = errors.New("двухфакторный вход уже включён")
	ErrMFANotPending  = errors.New("сначала начните настройку: секрета для подтверждения нет")
	ErrBadCode        = errors.New("неверный код")
)

const recoveryCodes = 10

// MFAStatus is what the settings screen shows.
type MFAStatus struct {
	Enabled      bool `json:"enabled"`
	Pending      bool `json:"pending"`
	RecoveryLeft int  `json:"recovery_left"`
}

func (s *Store) mfaReady() bool { return s.Box != nil }

// HasMFA reports whether the account must give a second factor to sign in.
func (s *Store) HasMFA(userID int64) (bool, error) {
	if !s.mfaReady() {
		return false, nil
	}
	var enabled int
	err := s.DB.QueryRow(`SELECT enabled FROM user_totp WHERE user_id = ?`, userID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled != 0, err
}

// MFAStatus returns the account's state.
func (s *Store) MFAStatus(userID int64) (MFAStatus, error) {
	var st MFAStatus
	if !s.mfaReady() {
		return st, ErrMFAUnavailable
	}
	var enabled int
	err := s.DB.QueryRow(`SELECT enabled FROM user_totp WHERE user_id = ?`, userID).Scan(&enabled)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return st, nil
	case err != nil:
		return st, err
	}
	st.Enabled, st.Pending = enabled != 0, enabled == 0
	if st.Enabled {
		err = s.DB.QueryRow(`SELECT COUNT(*) FROM user_recovery_code WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&st.RecoveryLeft)
	}
	return st, err
}

// BeginMFA mints a secret for the account and keeps it pending until a code from it is confirmed.
// A pending secret is replaced by a new request; an enabled one is not (turn it off first).
func (s *Store) BeginMFA(userID int64) (secret string, err error) {
	if !s.mfaReady() {
		return "", ErrMFAUnavailable
	}
	if on, err := s.HasMFA(userID); err != nil {
		return "", err
	} else if on {
		return "", ErrMFAEnabled
	}
	secret, err = NewTOTPSecret()
	if err != nil {
		return "", err
	}
	sealed, err := s.Box.Seal(secret)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec(`
		INSERT INTO user_totp (user_id, secret_enc, enabled, last_step, created_at) VALUES (?, ?, 0, 0, ?)
		ON CONFLICT (user_id) DO UPDATE SET secret_enc = excluded.secret_enc, enabled = 0, last_step = 0, created_at = excluded.created_at`,
		userID, sealed, timeutil.ToDBUTC(timeutil.NowUTC()))
	return secret, err
}

// loadSecret opens the account's secret and returns it with the newest accepted step.
func (s *Store) loadSecret(userID int64) (secret string, enabled bool, lastStep int64, err error) {
	var sealed string
	var en int
	err = s.DB.QueryRow(`SELECT secret_enc, enabled, last_step FROM user_totp WHERE user_id = ?`, userID).Scan(&sealed, &en, &lastStep)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, 0, ErrMFANotPending
	}
	if err != nil {
		return "", false, 0, err
	}
	secret, err = s.Box.Open(sealed)
	return secret, en != 0, lastStep, err
}

// acceptStep records step as used; false means the same or a newer step was already accepted (replay).
func (s *Store) acceptStep(userID, step int64) (bool, error) {
	res, err := s.DB.Exec(`UPDATE user_totp SET last_step = ? WHERE user_id = ? AND last_step < ?`, step, userID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ConfirmMFA turns two-factor on once the account proves it holds the secret, and returns the
// recovery codes (shown once).
func (s *Store) ConfirmMFA(userID int64, code string) ([]string, error) {
	if !s.mfaReady() {
		return nil, ErrMFAUnavailable
	}
	secret, enabled, _, err := s.loadSecret(userID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, ErrMFAEnabled
	}
	step, ok := MatchTOTP(secret, code, timeutil.NowUTC())
	if !ok {
		return nil, ErrBadCode
	}
	if _, err := s.acceptStep(userID, step); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`UPDATE user_totp SET enabled = 1 WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}
	return s.newRecoveryCodes(userID)
}

// VerifySecondFactor checks a TOTP code or a recovery code at sign-in (or before a sensitive
// change). A TOTP code works once; a recovery code is spent.
func (s *Store) VerifySecondFactor(userID int64, code string) error {
	if !s.mfaReady() {
		return ErrMFAUnavailable
	}
	code = strings.TrimSpace(code)
	secret, enabled, last, err := s.loadSecret(userID)
	if errors.Is(err, secretbox.ErrBadKey) && enabled {
		// The sealed secret does not open (the server key was lost or replaced): a recovery
		// code, which does not depend on it, still gets the person in to re-enrol.
		return s.spendRecoveryCode(userID, code)
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrMFANotPending
	}
	if step, ok := MatchTOTP(secret, code, timeutil.NowUTC()); ok {
		if step <= last {
			return ErrBadCode
		}
		if fresh, err := s.acceptStep(userID, step); err != nil {
			return err
		} else if !fresh {
			return ErrBadCode
		}
		return nil
	}
	return s.spendRecoveryCode(userID, code)
}

// DisableMFA removes the second factor (the caller verified a code first).
func (s *Store) DisableMFA(userID int64) error {
	if !s.mfaReady() {
		return ErrMFAUnavailable
	}
	if _, err := s.DB.Exec(`DELETE FROM user_recovery_code WHERE user_id = ?`, userID); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM user_totp WHERE user_id = ?`, userID)
	return err
}

// RegenerateRecoveryCodes replaces the unused codes with a fresh set.
func (s *Store) RegenerateRecoveryCodes(userID int64) ([]string, error) {
	if !s.mfaReady() {
		return nil, ErrMFAUnavailable
	}
	return s.newRecoveryCodes(userID)
}

const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789" // no look-alikes (i l o 0 1)

func newRecoveryCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 0, 11)
	for i, b := range raw {
		if i == 5 {
			out = append(out, '-')
		}
		out = append(out, recoveryAlphabet[int(b)%len(recoveryAlphabet)])
	}
	return string(out), nil
}

func normRecovery(code string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
}

func (s *Store) newRecoveryCodes(userID int64) ([]string, error) {
	if _, err := s.DB.Exec(`DELETE FROM user_recovery_code WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	codes := make([]string, 0, recoveryCodes)
	for i := 0; i < recoveryCodes; i++ {
		c, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.DB.Exec(`INSERT INTO user_recovery_code (user_id, code_hash, created_at) VALUES (?, ?, ?)`,
			userID, tokens.Hash(normRecovery(c)), now); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

func (s *Store) spendRecoveryCode(userID int64, code string) error {
	res, err := s.DB.Exec(`UPDATE user_recovery_code SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		timeutil.ToDBUTC(timeutil.NowUTC()), userID, tokens.Hash(normRecovery(code)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadCode
	}
	return nil
}

// ResetMFA is the host-side escape hatch (quests-server 2fa-reset): drop the second factor and
// every session of the account.
func (s *Store) ResetMFA(username string) error {
	var id int64
	if err := s.DB.QueryRow(`SELECT id FROM appuser WHERE username = ?`, normUsername(username)).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoUser
		}
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM user_recovery_code WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM user_totp WHERE user_id = ?`, id); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM usersession WHERE user_id = ?`, id)
	return err
}
