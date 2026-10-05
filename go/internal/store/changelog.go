package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/valirum/quests/go/internal/timeutil"
)

func (s *Store) QuestIDForStep(ctx context.Context, stepID int64) (int64, error) {
	var qid sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT quest_id FROM queststep WHERE id=?`, stepID).Scan(&qid)
	if err == sql.ErrNoRows || !qid.Valid {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return qid.Int64, nil
}

func (s *Store) ListChangeLog(ctx context.Context, limit int, questID *int64, beforeID *int64) ([]map[string]any, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	q := `SELECT id, at, kind, quest_id, title, detail, significance, revision,
			entity_type, entity_id, parent_type, parent_id
		FROM questchangelog WHERE 1=1`
	args := []any{}
	if questID != nil {
		q += ` AND quest_id=?`
		args = append(args, *questID)
	}
	if beforeID != nil {
		q += ` AND id < ?`
		args = append(args, *beforeID)
	}
	q += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var at, kind, title, detail string
		var nqid, nrev sql.NullInt64
		var nsig, nptype sql.NullString
		var etype string
		var neid, npid sql.NullInt64
		if err := rows.Scan(&id, &at, &kind, &nqid, &title, &detail, &nsig, &nrev,
			&etype, &neid, &nptype, &npid); err != nil {
			return nil, err
		}
		t, _ := timeutil.ParseFlexible(at)
		m := map[string]any{
			"id": id, "at": derefISO(&t), "kind": kind, "quest_id": nil,
			"title": title, "detail": detail, "significance": nil, "revision": nil,
			"entity_type": etype, "entity_id": nil, "parent_type": nil, "parent_id": nil,
		}
		if neid.Valid {
			m["entity_id"] = neid.Int64
		}
		if nptype.Valid {
			m["parent_type"] = nptype.String
		}
		if npid.Valid {
			m["parent_id"] = npid.Int64
		}
		if nqid.Valid {
			m["quest_id"] = nqid.Int64
		}
		if nsig.Valid {
			m["significance"] = nsig.String
		}
		if nrev.Valid {
			m["revision"] = int(nrev.Int64)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ChangeEvent is a log entry about a non-quest entity (note, attachment).
// Quest events are staged inside the quest transaction instead.
type ChangeEvent struct {
	Kind       string
	EntityType string // note | attachment
	EntityID   int64
	ParentType string // owner of an attachment; empty otherwise
	ParentID   int64
	Title      string
	Detail     string
	// Coalesce folds the entry into the previous identical (kind, entity) one
	// when that is newer than this window, so autosave bursts stay one line.
	Coalesce time.Duration
}

// LogEvent appends e to the change log. Failures are the caller's to ignore:
// history must never fail the write it describes.
func (s *Store) LogEvent(ctx context.Context, e ChangeEvent) error {
	if len(e.Title) > 200 {
		e.Title = e.Title[:200]
	}
	if len(e.Detail) > 500 {
		e.Detail = e.Detail[:500]
	}
	now := timeutil.NowUTC()
	if e.Coalesce > 0 {
		res, err := s.DB.ExecContext(ctx, `
			UPDATE questchangelog SET at=?, title=?, detail=?
			WHERE id = (
				SELECT id FROM questchangelog
				WHERE kind=? AND entity_type=? AND entity_id=? AND at >= ?
				ORDER BY id DESC LIMIT 1)`,
			timeutil.ToDBUTC(now), e.Title, e.Detail, e.Kind, e.EntityType, e.EntityID,
			timeutil.ToDBUTC(now.Add(-e.Coalesce)))
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				return nil
			}
		}
	}
	var ptype, pid any
	if e.ParentType != "" {
		ptype, pid = e.ParentType, e.ParentID
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO questchangelog (at, kind, quest_id, title, detail, entity_type, entity_id, parent_type, parent_id)
		VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?)`,
		timeutil.ToDBUTC(now), e.Kind, e.Title, e.Detail, e.EntityType, e.EntityID, ptype, pid)
	return err
}
