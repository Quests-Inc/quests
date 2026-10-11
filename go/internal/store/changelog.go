package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
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

// ChangeLogFilter narrows ListChangeLog. Zero value = everything of entity
// type quest (what the HUD expects); set EntityTypes to widen.
type ChangeLogFilter struct {
	Limit       int
	QuestID     *int64   // the quest itself, or an attachment owned by it
	EntityTypes []string // quest | note | attachment; empty = quest only, "all" = any
	EntityID    *int64   // with exactly one EntityType: one entity's history
	OwnerType   string   // with OwnerID: the entity itself plus everything attached to it
	OwnerID     *int64
	Kinds       []string
	QuestlineID *int64 // quests of the line, their attachments, and its own attachments
	CategoryID  *int64
	TagID       *int64
	From, To    *time.Time // [From, To)
	Query       string     // case-insensitive substring of title/detail
	BeforeID    *int64
	BeforeAt    *time.Time // with BeforeID: keyset cursor on (at, id)
}

func (s *Store) ListChangeLog(ctx context.Context, f ChangeLogFilter) ([]map[string]any, error) {
	limit := f.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := strings.ToLower(strings.TrimSpace(f.Query))

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, a ...any) {
		where = append(where, cond)
		args = append(args, a...)
	}
	questIn := func(sub string, a ...any) {
		add("(quest_id IN ("+sub+") OR (parent_type='quest' AND parent_id IN ("+sub+")))", append(append([]any{}, a...), a...)...)
	}

	types := f.EntityTypes
	all := false
	for _, t := range types {
		if t == "all" {
			all = true
		}
	}
	switch {
	case all:
	case len(types) == 0:
		add("entity_type='quest'")
	default:
		ph := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
		anyTypes := make([]any, len(types))
		for i, t := range types {
			anyTypes[i] = t
		}
		add("entity_type IN ("+ph+")", anyTypes...)
	}
	if f.EntityID != nil {
		add("entity_id=?", *f.EntityID)
	}
	if f.OwnerType != "" && f.OwnerID != nil {
		add("((entity_type=? AND entity_id=?) OR (parent_type=? AND parent_id=?))", f.OwnerType, *f.OwnerID, f.OwnerType, *f.OwnerID)
	}
	if f.QuestID != nil {
		add("(quest_id=? OR (parent_type='quest' AND parent_id=?))", *f.QuestID, *f.QuestID)
	}
	if len(f.Kinds) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(f.Kinds)), ",")
		ks := make([]any, len(f.Kinds))
		for i, k := range f.Kinds {
			ks[i] = k
		}
		add("kind IN ("+ph+")", ks...)
	}
	if f.QuestlineID != nil {
		where = append(where, "((quest_id IN (SELECT id FROM quest WHERE questline_id=?)) OR (parent_type='quest' AND parent_id IN (SELECT id FROM quest WHERE questline_id=?)) OR (parent_type='questline' AND parent_id=?))")
		args = append(args, *f.QuestlineID, *f.QuestlineID, *f.QuestlineID)
	}
	if f.CategoryID != nil {
		questIn("SELECT id FROM quest WHERE category_id=?", *f.CategoryID)
	}
	if f.TagID != nil {
		questIn("SELECT quest_id FROM quest_tag WHERE tag_id=?", *f.TagID)
	}
	if f.From != nil {
		add("at >= ?", timeutil.ToDBUTC(*f.From))
	}
	if f.To != nil {
		add("at < ?", timeutil.ToDBUTC(*f.To))
	}

	cursorAt, cursorID := f.BeforeAt, f.BeforeID
	out := make([]map[string]any, 0, limit)
	for len(out) < limit {
		w := append([]string{}, where...)
		a := append([]any{}, args...)
		if cursorID != nil {
			if cursorAt != nil {
				w = append(w, "(at < ? OR (at = ? AND id < ?))")
				a = append(a, timeutil.ToDBUTC(*cursorAt), timeutil.ToDBUTC(*cursorAt), *cursorID)
			} else {
				w = append(w, "id < ?")
				a = append(a, *cursorID)
			}
		}
		page := limit
		if query != "" {
			page = 500
		}
		a = append(a, page)
		rows, err := s.DB.QueryContext(ctx, `SELECT id, at, kind, quest_id, title, detail, significance, revision,
			entity_type, entity_id, parent_type, parent_id
			FROM questchangelog WHERE `+strings.Join(w, " AND ")+` ORDER BY at DESC, id DESC LIMIT ?`, a...)
		if err != nil {
			return nil, err
		}
		batch, lastID, lastAt, err := scanChangeRows(rows)
		if err != nil {
			return nil, err
		}
		for _, m := range batch {
			if query != "" {
				hay := strings.ToLower(m["title"].(string) + " " + m["detail"].(string))
				if !strings.Contains(hay, query) {
					continue
				}
			}
			out = append(out, m)
			if len(out) == limit {
				return out, nil
			}
		}
		if len(batch) < page {
			break
		}
		cursorID, cursorAt = &lastID, &lastAt
	}
	return out, nil
}

func scanChangeRows(rows *sql.Rows) ([]map[string]any, int64, time.Time, error) {
	defer rows.Close()
	out := []map[string]any{}
	var lastID int64
	var lastAt time.Time
	for rows.Next() {
		var id int64
		var at, kind, title, detail string
		var nqid, nrev sql.NullInt64
		var nsig, nptype sql.NullString
		var etype string
		var neid, npid sql.NullInt64
		if err := rows.Scan(&id, &at, &kind, &nqid, &title, &detail, &nsig, &nrev,
			&etype, &neid, &nptype, &npid); err != nil {
			return nil, 0, time.Time{}, err
		}
		t, _ := timeutil.ParseFlexible(at)
		lastID, lastAt = id, t
		m := map[string]any{
			"id": id, "at": derefISO(&t), "kind": kind, "quest_id": nil,
			"title": title, "detail": detail, "significance": nil, "revision": nil,
			"entity_type": etype, "entity_id": nil, "parent_type": nil, "parent_id": nil,
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
		if neid.Valid {
			m["entity_id"] = neid.Int64
		}
		if nptype.Valid {
			m["parent_type"] = nptype.String
		}
		if npid.Valid {
			m["parent_id"] = npid.Int64
		}
		out = append(out, m)
	}
	return out, lastID, lastAt, rows.Err()
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
