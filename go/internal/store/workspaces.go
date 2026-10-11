package store

import (
	"context"
	"database/sql"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/tokens"
	"github.com/valirum/quests/go/internal/platform/wsctx"
)

// Identity gives access to accounts, workspaces and roles (the identity context) over the same database.
// Transitional: until every context has its own service injected by the composition root (ADR-0001),
// the pieces that still live in store reach identity through this accessor.
func (s *Store) Identity() *identity.Store { return &identity.Store{DB: s.DB} }

// CopyCategories gives workspace `to` the category set of workspace `from` (a fresh workspace is usable at once).
func (s *Store) CopyCategories(ctx context.Context, from, to int64) error {
	type cat struct {
		slug, label, color string
		sort               int
	}
	rows, err := s.DB.QueryContext(wsctx.With(ctx, from), `SELECT slug, label, sort_order, color FROM questcategory ORDER BY sort_order, id`)
	if err != nil {
		return err
	}
	var cats []cat
	for rows.Next() {
		var c cat
		if err := rows.Scan(&c.slug, &c.label, &c.sort, &c.color); err != nil {
			rows.Close()
			return err
		}
		cats = append(cats, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	toCtx := wsctx.With(ctx, to)
	for _, c := range cats {
		if _, err := s.DB.ExecContext(toCtx,
			`INSERT INTO questcategory (slug, label, sort_order, created_at, color) VALUES (?, ?, ?, ?, ?)`,
			c.slug, c.label, c.sort, timeutil.ToDBUTC(timeutil.NowUTC()), c.color); err != nil {
			return err
		}
	}
	return nil
}

// RunnerWorkspaceByToken resolves the workspace of a runner's permanent token
// (the runner has not been placed in a workspace yet, so this goes through a
// SECURITY DEFINER function that bypasses row-level security).
func (s *Store) RunnerWorkspaceByToken(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrRunnerAuth
	}
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_token(?)`, tokens.Hash(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerAuth
	}
	return id.Int64, nil
}

// RunnerWorkspaceByReg is the same for a one-time registration token.
func (s *Store) RunnerWorkspaceByReg(ctx context.Context, token string) (int64, error) {
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_reg(?)`, tokens.Hash(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerToken
	}
	return id.Int64, nil
}

// guardAuthor refuses a command written by someone who is not an owner of the workspace (kernel.CommandAuthor).
func (s *Store) guardAuthor(ctx context.Context, command, old *string) error {
	return kernel.CommandAuthor(ctx, s.Identity(), command, old)
}

// guardExec refuses a command that has no runner (kernel.CommandExec).
func (s *Store) guardExec(_ context.Context, command, runOn *string) error {
	return kernel.CommandExec(command, runOn)
}

// Knowledge gives access to notes, attachments and search (the knowledge context) over the same database.
// Transitional, like Identity: until the composition root injects each context's service (ADR-0001).
func (s *Store) Knowledge() *knowledge.Store { return &knowledge.Store{DB: s.DB} }
