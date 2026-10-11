package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// Stats are instance-wide aggregates for monitoring. They deliberately carry no
// workspace identity: they are summed over every workspace.
type Stats struct {
	Workspaces      int
	QuestsByStatus  map[string]int
	Notes           int
	RunnerJobsState map[string]int
	RunnersActive   int
	Attachments     int
	AttachmentBytes int64
	// AttachmentBytesMax is the largest single workspace, revisions included (quota headroom).
	AttachmentBytesMax int64
	LastBackupAt       int64 // unix seconds of the newest successful backup (0 = none)
	LastBackupBytes    int64
}

// AggregateStats sums the counters over all workspaces (each one read under its own context).
func (s *Store) AggregateStats(ctx context.Context) (Stats, error) {
	st := Stats{QuestsByStatus: map[string]int{}, RunnerJobsState: map[string]int{}}
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	err := s.ForEachWorkspace(ctx, func(wctx context.Context, _ int64) {
		st.Workspaces++
		rows, err := s.DB.QueryContext(wctx, `SELECT status, COUNT(*) FROM quest GROUP BY status`)
		note(err)
		if err == nil {
			for rows.Next() {
				var k string
				var n int
				note(rows.Scan(&k, &n))
				st.QuestsByStatus[k] += n
			}
			rows.Close()
		}
		rows, err = s.DB.QueryContext(wctx, `SELECT state, COUNT(*) FROM runner_job GROUP BY state`)
		note(err)
		if err == nil {
			for rows.Next() {
				var k string
				var n int
				note(rows.Scan(&k, &n))
				st.RunnerJobsState[k] += n
			}
			rows.Close()
		}
		var n int
		var b sql.NullInt64
		note(s.DB.QueryRowContext(wctx, `SELECT COUNT(*) FROM note`).Scan(&n))
		st.Notes += n
		note(s.DB.QueryRowContext(wctx, `SELECT COUNT(*) FROM runner WHERE revoked = 0`).Scan(&n))
		st.RunnersActive += n
		note(s.DB.QueryRowContext(wctx, `SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM attachment`).Scan(&n, &b))
		st.Attachments += n
		st.AttachmentBytes += b.Int64
		if used, uerr := s.AttachmentUsage(wctx); uerr == nil && used > st.AttachmentBytesMax {
			st.AttachmentBytesMax = used
		} else {
			note(uerr)
		}
	})
	note(err)
	// Backups are instance-level (backuplog is not per workspace).
	logs, err := s.ListBackupLogs(ctx)
	note(err)
	for _, l := range logs { // newest first
		if l.Error == "" && l.SizeBytes > 0 {
			if t, perr := parseDBTime(l.CreatedAt); perr == nil {
				st.LastBackupAt, st.LastBackupBytes = t, l.SizeBytes
				break
			}
		}
	}
	return st, firstErr
}

func parseDBTime(s string) (int64, error) {
	var t time.Time
	t, err := timeutil.ParseFlexible(s)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}
