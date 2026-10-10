package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/timeutil"
	"github.com/valirum/quests/go/internal/wsctx"
)

// DefaultWorkspaceID is where everything lived before workspaces existed.
const DefaultWorkspaceID int64 = 1

var ErrWorkspaceName = errors.New("workspace name must not be empty")

type Workspace struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Role      string    `json:"role,omitempty"`
	// Trusted workspaces may run commands on the server (see ServerExecAllowed).
	Trusted bool `json:"trusted"`
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
		SELECT w.id, w.name, w.created_at, m.role, w.trusted
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
		var trusted int
		if err := rows.Scan(&w.ID, &w.Name, &created, &w.Role, &trusted); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = timeutil.ParseFlexible(created)
		w.Trusted = trusted != 0
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

// CreateWorkspace makes a new workspace owned by userID. Its category set is
// copied from copyFrom (0 = start empty) so the new space is usable at once.
// MaxOwnedWorkspaces bounds how many live workspaces one account may own, so an account
// (invited ones included) cannot fill the database with workspaces and imports.
const MaxOwnedWorkspaces = 10

var ErrWorkspaceLimit = errors.New("достигнут предел числа пространств у одного аккаунта")

func (s *Store) CreateWorkspace(ctx context.Context, userID int64, name string, copyFrom int64) (Workspace, error) {
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
	if copyFrom > 0 {
		if err := s.copyCategories(ctx, copyFrom, id); err != nil {
			return Workspace{}, err
		}
	}
	return Workspace{ID: id, Name: name, CreatedAt: now, Role: "owner"}, nil
}

func (s *Store) copyCategories(ctx context.Context, from, to int64) error {
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

// EnsureUserWorkspace gives a user without any workspace one: the seeded
// workspace 1 if nobody owns it yet (the first account of an instance), else a
// fresh personal workspace with the default categories.
func (s *Store) EnsureUserWorkspace(ctx context.Context, userID int64, username string) (Workspace, error) {
	have, err := s.UserWorkspaces(ctx, userID)
	if err != nil {
		return Workspace{}, err
	}
	if len(have) > 0 {
		return have[0], nil
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_member WHERE workspace_id = ?`, DefaultWorkspaceID).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, err
	}
	if n == 0 {
		if err := s.AddWorkspaceMember(ctx, DefaultWorkspaceID, userID, "owner"); err != nil {
			return Workspace{}, err
		}
		return Workspace{ID: DefaultWorkspaceID, Name: "Основное", Role: "owner"}, nil
	}
	return s.CreateWorkspace(ctx, userID, username, DefaultWorkspaceID)
}

// AllWorkspaces lists every workspace (for trusted callers with no account).
func (s *Store) AllWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, created_at, trusted FROM workspace WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		var created string
		var trusted int
		if err := rows.Scan(&w.ID, &w.Name, &created, &trusted); err != nil {
			return nil, err
		}
		w.Trusted = trusted != 0
		w.CreatedAt, _ = timeutil.ParseFlexible(created)
		out = append(out, w)
	}
	return out, rows.Err()
}

// RunnerWorkspaceByToken resolves the workspace of a runner's permanent token
// (the runner has not been placed in a workspace yet, so this goes through a
// SECURITY DEFINER function that bypasses row-level security).
func (s *Store) RunnerWorkspaceByToken(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrRunnerAuth
	}
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_token(?)`, auth.HashToken(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerAuth
	}
	return id.Int64, nil
}

// RunnerWorkspaceByReg is the same for a one-time registration token.
func (s *Store) RunnerWorkspaceByReg(ctx context.Context, token string) (int64, error) {
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_reg(?)`, auth.HashToken(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerToken
	}
	return id.Int64, nil
}

// ErrServerExec is returned when a command that would run on the server is
// saved in (or reaches execution from) a workspace that is not trusted.
var ErrServerExec = errors.New("команды на сервере в этом пространстве запрещены — выберите устройство (раннер)")

// ServerExecAllowed reports whether the context's workspace may run commands on
// the server. Without PostgreSQL there is one implicit, trusted workspace.
func (s *Store) ServerExecAllowed(ctx context.Context) bool {
	id := wsctx.From(ctx)
	if id <= 0 {
		return false
	}
	var trusted int
	if err := s.DB.QueryRowContext(ctx, `SELECT trusted FROM workspace WHERE id = ? AND deleted_at IS NULL`, id).Scan(&trusted); err != nil {
		return false // unknown = not trusted
	}
	return trusted != 0
}

// SetWorkspaceTrusted flips the flag; callable from the host CLI only.
func (s *Store) SetWorkspaceTrusted(ctx context.Context, id int64, trusted bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE workspace SET trusted = ? WHERE id = ?`, boolInt(trusted), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrCommandOwner is returned when a member (not an owner) sets a command.
var ErrCommandOwner = errors.New("команды проверок и источников могут задавать только владельцы пространства")

// guardAuthor refuses a command written by someone who is not an owner of the
// workspace. A command runs with the authority of the machine that executes it
// (the server, or a runner the owner registered), so authoring one is an owner's
// right. old is the command already stored (nil for a new one): leaving it
// unchanged is fine, so a member can still edit the rest of a step or template.
func (s *Store) guardAuthor(ctx context.Context, command, old *string) error {
	if command == nil || strings.TrimSpace(*command) == "" {
		return nil
	}
	if old != nil && strings.TrimSpace(*old) == strings.TrimSpace(*command) {
		return nil
	}
	uid := wsctx.UserFrom(ctx)
	if uid == 0 {
		return nil // background work, auth off
	}
	if uid == wsctx.ActorAutomation {
		return ErrCommandOwner
	}
	role, err := s.MemberRole(ctx, wsctx.From(ctx), uid)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrCommandOwner
	}
	return nil
}

// guardExec refuses a command that would execute on the server (non-empty
// command, no runner) in an untrusted workspace.
func (s *Store) guardExec(ctx context.Context, command, runOn *string) error {
	if command == nil || strings.TrimSpace(*command) == "" {
		return nil
	}
	if runOn != nil && strings.TrimSpace(*runOn) != "" {
		return nil
	}
	if s.ServerExecAllowed(ctx) {
		return nil
	}
	return ErrServerExec
}

// --- rename / delete (quest=347) ---

// WorkspacePurgeAfter is how long a deleted workspace can still be restored.
const WorkspacePurgeAfter = 7 * 24 * time.Hour

var ErrWorkspaceTrusted = errors.New("доверенное пространство нельзя удалить: сначала снимите доверие на сервере (workspace untrust)")

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
// working) and schedules the purge. Trusted workspaces are refused: untrust them
// from the host first, so the operator's own space is never one click away from gone.
func (s *Store) DeleteWorkspace(ctx context.Context, id int64) (purgeAt time.Time, err error) {
	var trusted int
	if err := s.DB.QueryRowContext(ctx, `SELECT trusted FROM workspace WHERE id = ? AND deleted_at IS NULL`, id).Scan(&trusted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, err
	}
	if trusted != 0 {
		return time.Time{}, ErrWorkspaceTrusted
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
