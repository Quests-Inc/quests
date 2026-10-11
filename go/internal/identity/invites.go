package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/tokens"
)

var (
	ErrInviteInvalid = errors.New("invitation is invalid, used or expired")
	ErrLastOwner     = errors.New("a workspace needs at least one owner")
)

// Workspace roles.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Member struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// MemberRole returns the user's role in the workspace ("" = not a member).
func (s *Store) MemberRole(ctx context.Context, workspaceID, userID int64) (string, error) {
	var role string
	err := s.DB.QueryRowContext(ctx, `SELECT m.role FROM workspace_member m JOIN workspace w ON w.id = m.workspace_id
		 WHERE m.workspace_id = ? AND m.user_id = ? AND w.deleted_at IS NULL`, workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (s *Store) WorkspaceMembers(ctx context.Context, workspaceID int64) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.user_id, u.username, m.role
		FROM workspace_member m JOIN appuser u ON u.id = m.user_id
		WHERE m.workspace_id = ? ORDER BY m.role, u.username`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RemoveWorkspaceMember drops a member, refusing to remove the last owner.
// Sessions and tokens pinned to the workspace fall back to the user's first membership.
func (s *Store) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID int64) error {
	role, err := s.MemberRole(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if role == "" {
		return ErrNotFound
	}
	if role == RoleOwner {
		var owners int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_member WHERE workspace_id = ? AND role = 'owner'`, workspaceID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM workspace_member WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID)
	return err
}

// CreateInvite mints a one-time invitation; the secret is returned once, only its hash is stored.
func (s *Store) CreateInvite(ctx context.Context, workspaceID, createdBy int64) (string, time.Time, error) {
	var live int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM workspace WHERE id = ? AND deleted_at IS NULL`, workspaceID).Scan(&live); err != nil {
		return "", time.Time{}, err
	}
	secret, hash, err := tokens.New()
	if err != nil {
		return "", time.Time{}, err
	}
	now := timeutil.NowUTC()
	exp := now.Add(InviteTTL)
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO workspace_invite (workspace_id, token_hash, role, created_by, created_at, expires_at)
		VALUES (?, ?, 'member', ?, ?, ?)`,
		workspaceID, hash, createdBy, timeutil.ToDBUTC(now), timeutil.ToDBUTC(exp))
	if err != nil {
		return "", time.Time{}, err
	}
	return secret, exp, nil
}

// InviteInfo describes a still-valid invitation.
func (s *Store) InviteInfo(ctx context.Context, secret string) (name string, expires time.Time, err error) {
	var exp string
	err = s.DB.QueryRowContext(ctx, `
		SELECT w.name, i.expires_at
		FROM workspace_invite i JOIN workspace w ON w.id = i.workspace_id
		WHERE i.token_hash = ? AND i.used_at IS NULL AND i.expires_at > ? AND w.deleted_at IS NULL`,
		tokens.Hash(secret), timeutil.ToDBUTC(timeutil.NowUTC())).Scan(&name, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrInviteInvalid
	}
	if err != nil {
		return "", time.Time{}, err
	}
	expires, _ = timeutil.ParseFlexible(exp)
	return name, expires, nil
}

// ClaimInvite atomically spends an invitation and adds userID to its workspace
// (an existing membership keeps its role). Returns the workspace id.
func (s *Store) ClaimInvite(ctx context.Context, secret string, userID int64) (int64, error) {
	var wsID int64
	var role string
	err := s.DB.QueryRowContext(ctx, `
		UPDATE workspace_invite SET used_at = ?, used_by = ?
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
		  AND workspace_id IN (SELECT id FROM workspace WHERE deleted_at IS NULL)
		RETURNING workspace_id, role`,
		timeutil.ToDBUTC(timeutil.NowUTC()), userID, tokens.Hash(secret), timeutil.ToDBUTC(timeutil.NowUTC())).Scan(&wsID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInviteInvalid
	}
	if err != nil {
		return 0, err
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO workspace_member (workspace_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT (workspace_id, user_id) DO NOTHING`, wsID, userID, role)
	return wsID, err
}
