package execution

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// EgressEnabled says whether the administrator opened outgoing HTTP for the workspace of ctx. Off by default; only the admin CLI
// writes the grant.
func (s *Store) EgressEnabled(ctx context.Context) bool {
	var on int
	err := s.DB.QueryRowContext(ctx, `SELECT enabled FROM egress_grant WHERE workspace_id = (SELECT app_workspace())`).Scan(&on)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	return on != 0
}

// SetEgressGrant is the admin CLI's: it runs as the database owner, so it names the workspace itself.
func (s *Store) SetEgressGrant(ctx context.Context, workspace int64, enabled bool) error {
	on := 0
	if enabled {
		on = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO egress_grant (workspace_id, enabled, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (workspace_id) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		workspace, on, timeutil.ToDBUTC(timeutil.NowUTC()))
	return err
}

// EgressGrants lists the workspaces the administrator opened (admin CLI, database owner).
func (s *Store) EgressGrants(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT workspace_id, enabled FROM egress_grant`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		var on int
		if err := rows.Scan(&id, &on); err != nil {
			return nil, err
		}
		out[id] = on != 0
	}
	return out, rows.Err()
}

// LogEgress writes one line; a failure to write it never stops the request.
func (s *Store) LogEgress(ctx context.Context, e egress.Entry) {
	var script any
	if e.ScriptID > 0 {
		script = e.ScriptID
	}
	_, _ = s.DB.ExecContext(ctx, `
		INSERT INTO egress_log (workspace_id, at, script_id, host, method, status, bytes, millis, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Workspace, timeutil.ToDBUTC(timeutil.NowUTC()), script, trimTo(e.Host, 253), e.Method, e.Status, e.Bytes, e.Millis, trimTo(e.Result, 200))
}

// EgressLine is a row of the log.
type EgressLine struct {
	At        time.Time `json:"at"`
	Workspace int64     `json:"workspace_id"`
	ScriptID  int64     `json:"script_id,omitempty"`
	Host      string    `json:"host"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	Bytes     int64     `json:"bytes"`
	Millis    int64     `json:"millis"`
	Result    string    `json:"result"`
}

// RecentEgress reads the newest lines (admin CLI, database owner: every workspace unless one is named).
func (s *Store) RecentEgress(ctx context.Context, workspace int64, limit int) ([]EgressLine, error) {
	if limit < 1 || limit > 1000 {
		limit = 50
	}
	q := `SELECT at, workspace_id, COALESCE(script_id, 0), host, method, status, bytes, millis, result FROM egress_log`
	args := []any{}
	if workspace > 0 {
		q += ` WHERE workspace_id = ?`
		args = append(args, workspace)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EgressLine
	for rows.Next() {
		var l EgressLine
		var at string
		if err := rows.Scan(&at, &l.Workspace, &l.ScriptID, &l.Host, &l.Method, &l.Status, &l.Bytes, &l.Millis, &l.Result); err != nil {
			return nil, err
		}
		l.At, _ = timeutil.ParseFlexible(at)
		out = append(out, l)
	}
	return out, rows.Err()
}

// PurgeEgress forgets lines older than the cutoff (the workspace of ctx).
func (s *Store) PurgeEgress(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM egress_log WHERE at < ?`, timeutil.ToDBUTC(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
