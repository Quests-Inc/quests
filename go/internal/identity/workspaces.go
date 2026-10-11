package identity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/platform/apperr"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
)

// ErrNotFound is the shared "no such thing" error (platform/apperr), so handlers recognise it from any context.
var ErrNotFound = apperr.ErrNotFound

var ErrWorkspaceName = errors.New("workspace name must not be empty")

type Workspace struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Role      string    `json:"role,omitempty"`
}

// WorkspaceIDs lists every workspace; background loops iterate it.
func (s *Store) WorkspaceIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM workspace WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ForEachWorkspace runs fn once per workspace with that workspace on the context.
func (s *Store) ForEachWorkspace(ctx context.Context, fn func(ctx context.Context, id int64)) error {
	ids, err := s.WorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fn(wsctx.With(ctx, id), id)
	}
	return nil
}

// UserWorkspaces lists the workspaces a user belongs to.
func (s *Store) UserWorkspaces(ctx context.Context, userID int64) ([]Workspace, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT w.id, w.name, w.created_at, m.role
		FROM workspace w JOIN workspace_member m ON m.workspace_id = w.id
		WHERE m.user_id = ? AND w.deleted_at IS NULL ORDER BY w.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		var created string
		if err := rows.Scan(&w.ID, &w.Name, &created, &w.Role); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = timeutil.ParseFlexible(created)
		out = append(out, w)
	}
	return out, rows.Err()
}

// AddWorkspaceMember adds (or updates the role of) a user in a workspace.
func (s *Store) AddWorkspaceMember(ctx context.Context, workspaceID, userID int64, role string) error {
	if role == "" {
		role = "owner"
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO workspace_member (workspace_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = excluded.role`,
		workspaceID, userID, role)
	return err
}

// MaxOwnedWorkspaces bounds how many live workspaces one account may own, so an account
// (invited ones included) cannot fill the database with workspaces and imports.
const MaxOwnedWorkspaces = 10

var ErrWorkspaceLimit = errors.New("достигнут предел числа пространств у одного аккаунта")

// CreateWorkspace makes a new, empty workspace owned by userID. Seeding it with data (the category set,
// say) belongs to the contexts that own that data; the caller composes the steps.
func (s *Store) CreateWorkspace(ctx context.Context, userID int64, name string) (Workspace, error) {
	var owned int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_member m JOIN workspace w ON w.id = m.workspace_id
		 WHERE m.user_id = ? AND m.role = 'owner' AND w.deleted_at IS NULL`, userID).Scan(&owned); err != nil {
		return Workspace{}, err
	}
	if owned >= MaxOwnedWorkspaces {
		return Workspace{}, ErrWorkspaceLimit
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, ErrWorkspaceName
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	now := timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO workspace (name, created_at) VALUES (?, ?)`, name, timeutil.ToDBUTC(now))
	if err != nil {
		return Workspace{}, err
	}
	id, _ := res.LastInsertId()
	if err := s.AddWorkspaceMember(ctx, id, userID, "owner"); err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: id, Name: name, CreatedAt: now, Role: "owner"}, nil
}

// EnsureUserWorkspace gives a user without any workspace one: the seeded
// workspace 1 if nobody owns it yet (the first account of an instance), else a
// fresh personal workspace (fresh = true: the caller seeds it, the default categories for one).
func (s *Store) EnsureUserWorkspace(ctx context.Context, userID int64, username string) (ws Workspace, fresh bool, err error) {
	have, err := s.UserWorkspaces(ctx, userID)
	if err != nil {
		return Workspace{}, false, err
	}
	if len(have) > 0 {
		return have[0], false, nil
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_member WHERE workspace_id = ?`, DefaultWorkspaceID).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, false, err
	}
	if n == 0 {
		if err := s.AddWorkspaceMember(ctx, DefaultWorkspaceID, userID, "owner"); err != nil {
			return Workspace{}, false, err
		}
		return Workspace{ID: DefaultWorkspaceID, Name: "Основное", Role: "owner"}, false, nil
	}
	ws, err = s.CreateWorkspace(ctx, userID, username)
	return ws, err == nil, err
}

// AllWorkspaces lists every workspace (for callers with no account).
func (s *Store) AllWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, created_at FROM workspace WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		var created string
		if err := rows.Scan(&w.ID, &w.Name, &created); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = timeutil.ParseFlexible(created)
		out = append(out, w)
	}
	return out, rows.Err()
}

var ErrMainWorkspace = errors.New("основное пространство нельзя удалить из интерфейса")

// RenameWorkspace changes the display name.
func (s *Store) RenameWorkspace(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrWorkspaceName
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE workspace SET name = ? WHERE id = ? AND deleted_at IS NULL`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// WorkspaceName returns the name of a live workspace.
func (s *Store) WorkspaceName(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.DB.QueryRowContext(ctx, `SELECT name FROM workspace WHERE id = ? AND deleted_at IS NULL`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// DeleteWorkspace hides a workspace at once (its tokens, runners and sessions stop
// working) and schedules the purge. The main workspace (the one the instance started with) is
// refused: it is never one click away from gone.
func (s *Store) DeleteWorkspace(ctx context.Context, id int64) (purgeAt time.Time, err error) {
	var live int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM workspace WHERE id = ? AND deleted_at IS NULL`, id).Scan(&live); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, err
	}
	if id == DefaultWorkspaceID {
		return time.Time{}, ErrMainWorkspace
	}
	now := timeutil.NowUTC()
	if _, err := s.DB.ExecContext(ctx, `UPDATE workspace SET deleted_at = ? WHERE id = ?`, timeutil.ToDBUTC(now), id); err != nil {
		return time.Time{}, err
	}
	return now.Add(WorkspacePurgeAfter), nil
}

// RestoreWorkspace undoes a deletion that has not been purged yet (host CLI).
func (s *Store) RestoreWorkspace(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE workspace SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type DeletedWorkspace struct {
	ID      int64
	Name    string
	PurgeAt time.Time
}

// DeletedWorkspaces lists workspaces awaiting their purge.
func (s *Store) DeletedWorkspaces(ctx context.Context) ([]DeletedWorkspace, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, deleted_at FROM workspace WHERE deleted_at IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeletedWorkspace
	for rows.Next() {
		var d DeletedWorkspace
		var at string
		if err := rows.Scan(&d.ID, &d.Name, &at); err != nil {
			return nil, err
		}
		t, _ := timeutil.ParseFlexible(at)
		d.PurgeAt = t.Add(WorkspacePurgeAfter)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DueForPurge returns deleted workspaces whose grace period is over.
func (s *Store) DueForPurge(ctx context.Context) ([]int64, error) {
	cutoff := timeutil.NowUTC().Add(-WorkspacePurgeAfter)
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM workspace WHERE deleted_at IS NOT NULL AND deleted_at < ? ORDER BY id`, timeutil.ToDBUTC(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PurgeWorkspace removes the workspace row; every table's workspace_id cascades.
func (s *Store) PurgeWorkspace(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM workspace WHERE id = ? AND deleted_at IS NOT NULL`, id)
	return err
}

// AbortWorkspace hard-deletes a workspace that was just created for an import that failed.
func (s *Store) AbortWorkspace(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM workspace WHERE id = ?`, id)
	return err
}
