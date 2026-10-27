package execution

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// The run log (quest=489): finished and waiting runs as people read them, newest first, with the titles of what started each
// run and what it read. The snapshot stays out of the list; GetRun has it.

// RunSummary is one line of the run log.
type RunSummary struct {
	ID           int64      `json:"id"`
	GraphRef     string     `json:"graph_ref"`
	ScriptID     *int64     `json:"script_id,omitempty"`
	ScriptTitle  string     `json:"script_title"`
	SourceKind   string     `json:"source_kind"`
	SourceID     int64      `json:"source_id"`
	SourceTitle  string     `json:"source_title"`
	SubjectType  string     `json:"subject_type"`
	SubjectID    int64      `json:"subject_id"`
	SubjectTitle string     `json:"subject_title"`
	State        string     `json:"state"`
	Error        string     `json:"error,omitempty"`
	Dry          bool       `json:"dry,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// RunFilter narrows the run log; zero fields do not filter. Before is a run id: the page that goes on from it.
type RunFilter struct {
	ScriptID    int64
	SourceKind  string
	SourceID    int64
	SubjectType string
	SubjectID   int64
	State       string
	Before      int64
	Limit       int
}

// titleOf is the SQL that names an entity of a kind column and an id column (empty when it is gone or hidden).
func titleOf(kindCol, idCol string) string {
	return `COALESCE(CASE ` + kindCol + `
		WHEN 'step' THEN (SELECT title FROM queststep WHERE id = ` + idCol + `)
		WHEN 'quest' THEN (SELECT title FROM quest WHERE id = ` + idCol + `)
		WHEN 'note' THEN (SELECT title FROM note WHERE id = ` + idCol + `)
		WHEN 'template' THEN (SELECT title FROM questtemplate WHERE id = ` + idCol + `)
		WHEN 'questline' THEN (SELECT title FROM questline WHERE id = ` + idCol + `)
		WHEN 'hook' THEN (SELECT COALESCE(NULLIF(name, ''), 'хук') FROM hook WHERE id = ` + idCol + `)
		WHEN 'embed' THEN (SELECT title FROM script WHERE id = ` + idCol + `)
		WHEN 'test' THEN (SELECT title FROM script WHERE id = ` + idCol + `)
		END, '')`
}

// ListRuns is a page of the run log.
func (s *Store) ListRuns(ctx context.Context, f RunFilter) ([]RunSummary, error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(cond string, v any) {
		where = append(where, cond)
		args = append(args, v)
	}
	if f.ScriptID > 0 {
		add("r.script_id = ?", f.ScriptID)
	}
	if f.SourceKind != "" {
		add("r.source_kind = ?", f.SourceKind)
	}
	if f.SourceID > 0 {
		add("r.source_id = ?", f.SourceID)
	}
	if f.SubjectType != "" {
		add("r.subject_type = ?", f.SubjectType)
	}
	if f.SubjectID > 0 {
		add("r.subject_id = ?", f.SubjectID)
	}
	if f.State != "" {
		add("r.state = ?", f.State)
	}
	if f.Before > 0 {
		add("r.id < ?", f.Before)
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.graph_ref, r.script_id, COALESCE((SELECT title FROM script WHERE id = r.script_id), ''),
		       r.source_kind, COALESCE(r.source_id, 0), `+titleOf("r.source_kind", "r.source_id")+`,
		       r.subject_type, r.subject_id, `+titleOf("r.subject_type", "r.subject_id")+`,
		       r.state, LEFT(r.error, 500), r.dry, r.created_at, r.finished_at
		FROM script_run r WHERE `+strings.Join(where, " AND ")+`
		ORDER BY r.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunSummary{}
	for rows.Next() {
		var (
			x        RunSummary
			script   sql.NullInt64
			dry      int
			created  string
			finished sql.NullString
		)
		if err := rows.Scan(&x.ID, &x.GraphRef, &script, &x.ScriptTitle, &x.SourceKind, &x.SourceID, &x.SourceTitle,
			&x.SubjectType, &x.SubjectID, &x.SubjectTitle, &x.State, &x.Error, &dry, &created, &finished); err != nil {
			return nil, err
		}
		if script.Valid {
			v := script.Int64
			x.ScriptID = &v
		}
		x.Dry = dry == 1
		x.CreatedAt, _ = timeutil.ParseFlexible(created)
		x.FinishedAt = parseNullTime(finished)
		out = append(out, x)
	}
	return out, rows.Err()
}

// SourceRunStats is how a source's runs went lately: the newest one and how many started in the last hour.
type SourceRunStats struct {
	LastID    int64     `json:"last_id"`
	LastState string    `json:"last_state"`
	LastAt    time.Time `json:"last_at"`
	LastError string    `json:"last_error,omitempty"`
	LastHour  int       `json:"last_hour"`
}

// RunStatsOf is the stats of every source of a kind that has runs (hooks for the hook list, steps for a quest's steps…).
func (s *Store) RunStatsOf(ctx context.Context, kind string) (map[int64]SourceRunStats, error) {
	hourAgo := timeutil.ToDBUTC(timeutil.NowUTC().Add(-time.Hour))
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.source_id, r.id, r.state, r.created_at, LEFT(r.error, 300),
		       (SELECT COUNT(*) FROM script_run c WHERE c.source_kind = r.source_kind AND c.source_id = r.source_id AND c.created_at >= ?)
		FROM script_run r
		WHERE r.source_kind = ? AND r.id = (SELECT MAX(id) FROM script_run m WHERE m.source_kind = r.source_kind AND m.source_id = r.source_id)`,
		hourAgo, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]SourceRunStats{}
	for rows.Next() {
		var (
			src     int64
			st      SourceRunStats
			created string
		)
		if err := rows.Scan(&src, &st.LastID, &st.LastState, &created, &st.LastError, &st.LastHour); err != nil {
			return nil, err
		}
		st.LastAt, _ = timeutil.ParseFlexible(created)
		out[src] = st
	}
	return out, rows.Err()
}
