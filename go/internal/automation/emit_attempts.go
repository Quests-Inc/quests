package automation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// emitAttemptKeep bounds the per-template attempt log.
const emitAttemptKeep = 200

// EmitAttempt is one emit_pool_command execution. Message must already be
// free of template secret values (see schedule.execEmitPoolCommand).
type EmitAttempt struct {
	TemplateID int64
	PeriodKey  string
	At         time.Time
	Attempt    int
	Status     string // ok | error | timeout | bad_json
	DurationMS int64
	Items      int
	Picked     int
	PickedRefs []string
	Message    string
}

// RecordEmitAttempt appends one row and trims the template's log to the most
// recent emitAttemptKeep entries.
func (s *Store) RecordEmitAttempt(ctx context.Context, a EmitAttempt) error {
	var refs any
	if len(a.PickedRefs) > 0 {
		b, err := json.Marshal(a.PickedRefs)
		if err != nil {
			return err
		}
		refs = string(b)
	}
	var msg any
	if a.Message != "" {
		msg = a.Message
	}
	if a.Attempt < 1 {
		a.Attempt = 1
	}
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO templateemitattempt
			(template_id, period_key, at, attempt, status, duration_ms, items, picked, picked_refs, message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TemplateID, a.PeriodKey, timeutil.ToDBUTC(a.At), a.Attempt, a.Status, a.DurationMS,
		a.Items, a.Picked, refs, msg); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM templateemitattempt
		WHERE template_id = ? AND id NOT IN (
			SELECT id FROM templateemitattempt WHERE template_id = ? ORDER BY id DESC LIMIT ?)`,
		a.TemplateID, a.TemplateID, emitAttemptKeep)
	return err
}

// ListEmitAttempts returns the template's attempts, newest first.
func (s *Store) ListEmitAttempts(ctx context.Context, templateID int64, limit int) ([]map[string]any, error) {
	if limit < 1 || limit > emitAttemptKeep {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, period_key, at, attempt, status, duration_ms, items, picked, picked_refs, message
		FROM templateemitattempt WHERE template_id = ? ORDER BY id DESC LIMIT ?`, templateID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, attempt, dur, items, picked int64
		var period, at, status string
		var refs, msg *string
		if err := rows.Scan(&id, &period, &at, &attempt, &status, &dur, &items, &picked, &refs, &msg); err != nil {
			return nil, err
		}
		atISO := at
		if t, err := timeutil.ParseFlexible(at); err == nil {
			if iso := timeutil.ToUTCISO(&t); iso != nil {
				atISO = *iso
			}
		}
		m := map[string]any{
			"id": id, "template_id": templateID, "period_key": period, "at": atISO,
			"attempt": attempt, "status": status, "duration_ms": dur,
			"items": items, "picked": picked, "picked_refs": []string{}, "message": "",
		}
		if refs != nil {
			var keys []string
			if json.Unmarshal([]byte(*refs), &keys) == nil {
				m["picked_refs"] = keys
			}
		}
		if msg != nil {
			m["message"] = *msg
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
