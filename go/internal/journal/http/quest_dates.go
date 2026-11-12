package journalhttp

import (
	"encoding/json"
	"errors"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"net/http"
)

// applyQuestDates handles the independent source, window and expiry options and lifecycle timestamps.
func (h *Handler) applyQuestDates(w http.ResponseWriter, r *http.Request, raw map[string]json.RawMessage, q *journal.Quest) bool {
	deadlineTouched, durationExplicit := false, false
	if v, ok := raw["deadline_at"]; ok {
		deadlineTouched = true
		if string(v) == "null" {
			q.DeadlineAt = nil
		} else {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				t, err := timeutil.ParseFlexible(s)
				if err != nil {
					writeErr(w, http.StatusUnprocessableEntity, "invalid deadline_at")
					return false
				}
				q.DeadlineAt = &t
			}
		}
	}
	if v, ok := raw["duration_seconds"]; ok {
		durationExplicit = true
		if string(v) == "null" {
			q.DurationSeconds = nil
		} else {
			var n int
			if err := json.Unmarshal(v, &n); err != nil || n < 0 {
				writeErr(w, 422, "duration_seconds must be nonnegative")
				return false
			}
			q.DurationSeconds = &n
		}
	}
	now := timeutil.NowUTC()
	q.UpdatedAt = now
	if q.Status == journal.StatusCompleted && q.CompletedAt == nil {
		q.CompletedAt = &now
	}
	if q.Status != journal.StatusCompleted {
		q.CompletedAt = nil
	}
	if v, ok := raw["expire_on_deadline"]; ok {
		var expires bool
		if err := json.Unmarshal(v, &expires); err != nil || string(v) == "null" {
			writeErr(w, 422, "expire_on_deadline must be boolean")
			return false
		}
		q.ExpireOnDeadline = &expires
	}
	requestedDuration := q.DurationSeconds
	if deadlineTouched || durationExplicit || (q.DeadlineAt != nil && q.DurationSeconds == nil) {
		duration := q.DurationSeconds
		if duration == nil {
			zero := 0
			duration = &zero
		}
		d, dur := timeutil.NormalizeDeadline(q.DeadlineAt, duration, true, now)
		q.DeadlineAt = d
		q.DurationSeconds = dur
	}
	eventValue, eventTouched := raw["event_id"]
	for _, key := range []string{"event_edge", "event_dir", "event_offset_seconds"} {
		if _, ok := raw[key]; ok {
			eventTouched = true
		}
	}
	if eventTouched {
		v := eventValue
		if v == nil && q.EventID != nil {
			v, _ = json.Marshal(*q.EventID)
		}
		if v == nil {
			writeErr(w, 422, "event_id is required for an event-relative deadline")
			return false
		}
		if string(v) == "null" {
			q.EventID = nil
			q.EventEdge = ""
			q.EventDir = ""
			q.EventOffset = nil
			q.EventTitle = nil
		} else {
			var id int64
			if err := json.Unmarshal(v, &id); err != nil || id <= 0 {
				writeErr(w, http.StatusUnprocessableEntity, "выберите событие")
				return false
			}
			edge, dir := q.EventEdge, q.EventDir
			offset := 0
			if q.EventOffset != nil {
				offset = *q.EventOffset
			}
			if ev, ok := raw["event_edge"]; ok {
				if err := json.Unmarshal(ev, &edge); err != nil {
					writeErr(w, 422, "event_edge must be a string")
					return false
				}
			}
			if dv, ok := raw["event_dir"]; ok {
				if err := json.Unmarshal(dv, &dir); err != nil {
					writeErr(w, 422, "event_dir must be a string")
					return false
				}
			}
			if ov, ok := raw["event_offset_seconds"]; ok && string(ov) != "null" {
				if err := json.Unmarshal(ov, &offset); err != nil {
					writeErr(w, 422, "event_offset_seconds must be an integer")
					return false
				}
			}
			if err := h.Store.ApplyEvent(r.Context(), q, &id, edge, dir, offset); err != nil {
				if errors.Is(err, journal.ErrNotFound) {
					writeErr(w, http.StatusUnprocessableEntity, "событие не найдено")
					return false
				}
				writeErr(w, http.StatusUnprocessableEntity, err.Error())
				return false
			}
		}
	}
	if eventTouched && q.EventID != nil {
		duration := requestedDuration
		if duration == nil {
			zero := 0
			duration = &zero
		}
		q.DeadlineAt, q.DurationSeconds = timeutil.NormalizeDeadline(q.DeadlineAt, duration, true, now)
	}
	if deadlineTouched && q.EventID != nil && !eventTouched {
		writeErr(w, 422, "clear event_id before setting a fixed deadline")
		return false
	}
	if (deadlineTouched || eventTouched) && q.Status == journal.StatusExpired && (q.DeadlineAt == nil || q.DeadlineAt.After(now)) {
		q.Status = journal.StatusActive
	}

	return true
}
