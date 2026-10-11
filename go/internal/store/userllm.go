package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// UserLLM is an account's assistant provider choice. The key is stored sealed
// (see internal/platform/secretbox); KeyHint is the visible tail, e.g. "…a1b2".
type UserLLM struct {
	Provider  string
	BaseURL   string
	Model     string
	KeySealed string
	KeyHint   string
}

// GetUserLLM returns the account's settings, or ErrNotFound.
func (s *Store) GetUserLLM(ctx context.Context, userID int64) (UserLLM, error) {
	var u UserLLM
	err := s.DB.QueryRowContext(ctx,
		`SELECT provider, base_url, model, api_key_enc, key_hint FROM user_llm WHERE user_id = ?`, userID,
	).Scan(&u.Provider, &u.BaseURL, &u.Model, &u.KeySealed, &u.KeyHint)
	if errors.Is(err, sql.ErrNoRows) {
		return UserLLM{}, ErrNotFound
	}
	return u, err
}

// SetUserLLM stores the settings; an empty KeySealed keeps the key already stored.
func (s *Store) SetUserLLM(ctx context.Context, userID int64, u UserLLM) error {
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO user_llm (user_id, provider, base_url, model, api_key_enc, key_hint, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			provider = excluded.provider, base_url = excluded.base_url, model = excluded.model,
			api_key_enc = CASE WHEN excluded.api_key_enc = '' THEN user_llm.api_key_enc ELSE excluded.api_key_enc END,
			key_hint = CASE WHEN excluded.api_key_enc = '' THEN user_llm.key_hint ELSE excluded.key_hint END,
			updated_at = excluded.updated_at`,
		userID, u.Provider, u.BaseURL, u.Model, u.KeySealed, u.KeyHint, now)
	return err
}

// DeleteUserLLM removes the settings and the key.
func (s *Store) DeleteUserLLM(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM user_llm WHERE user_id = ?`, userID)
	return err
}
