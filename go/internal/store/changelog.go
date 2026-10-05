package store

import (
	"context"
	"database/sql"

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
	q := `SELECT id, at, kind, quest_id, title, detail, significance, revision
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
		var nsig sql.NullString
		if err := rows.Scan(&id, &at, &kind, &nqid, &title, &detail, &nsig, &nrev); err != nil {
			return nil, err
		}
		t, _ := timeutil.ParseFlexible(at)
		m := map[string]any{
			"id": id, "at": derefISO(&t), "kind": kind, "quest_id": nil,
			"title": title, "detail": detail, "significance": nil, "revision": nil,
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
