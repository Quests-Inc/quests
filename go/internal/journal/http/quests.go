package journalhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/refs"
)

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListCategories(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []journal.Category{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) listQuests(w http.ResponseWriter, r *http.Request) {
	var f journal.ListFilter
	if st := r.URL.Query().Get("status"); st != "" {
		s := journal.QuestStatus(st)
		if !s.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, journal.InvalidStatusMsg(st))
			return
		}
		f.Status = &s
	}
	if p := r.URL.Query().Get("pinned"); p != "" {
		v := p == "1" || strings.EqualFold(p, "true")
		f.Pinned = &v
	}
	if ql := r.URL.Query().Get("questline_id"); ql != "" {
		if n, err := strconv.ParseInt(ql, 10, 64); err == nil {
			f.QuestlineID = &n
		}
	}
	quests, err := h.Store.ListQuests(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := timeutil.NowUTC()
	out := make([]journal.QuestRead, 0, len(quests))
	for _, q := range quests {
		out = append(out, journal.ToQuestRead(q, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := journal.ToQuestRead(q, timeutil.NowUTC())
	out.Refs = h.ResolveRefs(r, refs.Parse(q.Title+"\n"+q.Description))
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) createQuest(w http.ResponseWriter, r *http.Request) {
	var body journal.QuestCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "title required")
		return
	}
	if body.Status == "" {
		body.Status = journal.StatusActive
	}
	if !body.Status.Valid() {
		writeErr(w, http.StatusUnprocessableEntity, journal.InvalidStatusMsg(string(body.Status)))
		return
	}
	if body.Significance == "" {
		body.Significance = journal.SigCommon
	}
	if !body.Significance.Valid() {
		writeErr(w, http.StatusUnprocessableEntity, journal.InvalidSignificanceMsg(string(body.Significance)))
		return
	}
	now := timeutil.NowUTC()
	var deadline *time.Time
	if body.DeadlineAt != nil && strings.TrimSpace(*body.DeadlineAt) != "" {
		t, err := timeutil.ParseFlexible(*body.DeadlineAt)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "invalid deadline_at")
			return
		}
		deadline = &t
	}
	d, dur := timeutil.NormalizeDeadline(deadline, body.DurationSeconds, body.DurationSeconds != nil, now)
	catID, err := journal.ValidateCategoryID(r.Context(), h.Store, body.CategoryID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lineID := body.QuestlineID
	if lineID != nil {
		ok, err := h.Store.QuestlineExists(r.Context(), *lineID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "questline not found")
			return
		}
		if c, err := h.Store.QuestlineCategory(r.Context(), *lineID); err == nil && c != nil {
			catID = c
		}
	}
	q := journal.Quest{
		Title:           body.Title,
		Description:     body.Description,
		Status:          body.Status,
		Significance:    body.Significance,
		Pinned:          body.Pinned && !body.OverlayHidden,
		OverlayHidden:   body.OverlayHidden && !body.Pinned,
		SortOrder:       body.SortOrder,
		DeadlineAt:      d,
		DurationSeconds: dur,
		CategoryID:      catID,
		QuestlineID:     lineID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Automated:       body.Automated,
		Source:          httpx.RequestSource(r),
	}
	if len(body.Steps) == 0 {
		q.Steps = []journal.Step{{Title: body.Title, ProgressTotal: 1, SortOrder: 0}}
	} else {
		for i, sc := range body.Steps {
			ord := i
			if sc.SortOrder != nil {
				ord = *sc.SortOrder
			}
			cmd, iv := journal.NormalizeCheck(sc.CheckCommand, sc.CheckIntervalSeconds)
			st := journal.Step{
				Title: sc.Title, Description: sc.Description,
				ProgressCurrent: sc.ProgressCurrent, ProgressTotal: sc.ProgressTotal,
				SortOrder: ord, CheckCommand: cmd, CheckIntervalSeconds: iv,
				RunOn: kernel.NormalizeRunOn(sc.RunOn),
			}
			if !kernel.ValidRunOn(st.RunOn) {
				writeErr(w, http.StatusUnprocessableEntity, "run_on: "+kernel.ErrBadRunnerLabel.Error())
				return
			}
			if sc.WaitPrevious != nil {
				st.WaitPrevious = *sc.WaitPrevious
			}
			if sc.RunMode != nil {
				st.RunMode = journal.NormalizeRunMode(*sc.RunMode)
			}
			journal.NormalizeStepCheck(&st)
			if st.ProgressTotal < 1 {
				st.ProgressTotal = 1
			}
			q.Steps = append(q.Steps, st)
		}
	}
	created, err := h.Store.CreateQuest(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.TagIDs != nil {
		if err := h.Store.SetQuestTags(r.Context(), created.ID, body.TagIDs); err != nil {
			writeTagErr(w, err)
			return
		}
		created, err = h.Store.GetQuest(r.Context(), created.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	quiet, source := publishOpts(r)
	qid := created.ID
	h.Hub.Publish("quest_created", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &qid, Title: created.Title, Description: created.Description,
		Detail: "создано задание", Toast: !quiet, Source: source,
		Significance: string(created.Significance),
		Automated:    created.Automated,
	})
	writeJSON(w, http.StatusCreated, journal.ToQuestRead(created, timeutil.NowUTC()))
}

func (h *Handler) patchQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, ok := raw["steps"]; ok {
		writeErr(w, http.StatusUnprocessableEntity, "steps replace removed; use step CRUD")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	beforeStatus := q.Status
	deadlineTouched := false
	durationExplicit := false
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &q.Title)
	}
	if v, ok := raw["description"]; ok {
		_ = json.Unmarshal(v, &q.Description)
	}
	if v, ok := raw["status"]; ok {
		var st journal.QuestStatus
		_ = json.Unmarshal(v, &st)
		if !st.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, journal.InvalidStatusMsg(string(st)))
			return
		}
		q.Status = st
	}
	if v, ok := raw["significance"]; ok {
		var sg journal.Significance
		_ = json.Unmarshal(v, &sg)
		if !sg.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, journal.InvalidSignificanceMsg(string(sg)))
			return
		}
		q.Significance = sg
	}
	// pinned and overlay_hidden share one column, so turning one on turns the other off.
	if v, ok := raw["pinned"]; ok {
		_ = json.Unmarshal(v, &q.Pinned)
		if q.Pinned {
			q.OverlayHidden = false
		}
	}
	if v, ok := raw["overlay_hidden"]; ok {
		_ = json.Unmarshal(v, &q.OverlayHidden)
		if q.OverlayHidden {
			q.Pinned = false
		}
	}
	if v, ok := raw["sort_order"]; ok {
		_ = json.Unmarshal(v, &q.SortOrder)
	}
	if v, ok := raw["category_id"]; ok {
		var cid *int64
		_ = json.Unmarshal(v, &cid)
		cid, err = journal.ValidateCategoryID(r.Context(), h.Store, cid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		q.CategoryID = cid
	}
	if v, ok := raw["questline_id"]; ok {
		var lid *int64
		_ = json.Unmarshal(v, &lid)
		q.QuestlineID = lid
	}
	// A quest's category always follows its questline's category when one is
	// assigned — ignore a manually supplied category_id in that case, so the
	// two can never drift apart (regardless of which field this request touched).
	if q.QuestlineID != nil {
		if c, err := h.Store.QuestlineCategory(r.Context(), *q.QuestlineID); err == nil && c != nil {
			q.CategoryID = c
		}
	}
	if v, ok := raw["automated"]; ok {
		_ = json.Unmarshal(v, &q.Automated)
	}
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
					return
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
			_ = json.Unmarshal(v, &n)
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
	if deadlineTouched || durationExplicit || (q.DeadlineAt != nil && q.DurationSeconds == nil) {
		d, dur := timeutil.NormalizeDeadline(q.DeadlineAt, q.DurationSeconds, durationExplicit, now)
		q.DeadlineAt = d
		q.DurationSeconds = dur
	}
	kind := "quest_updated"
	detail := "изменено"
	toast := false
	if q.Status != beforeStatus {
		switch q.Status {
		case journal.StatusCompleted:
			kind, detail, toast = "quest_completed", "завершено", true
		case journal.StatusFailed:
			kind, detail, toast = "quest_failed", "провалено", true
		case journal.StatusExpired:
			kind, detail, toast = "quest_expired", "просрочено", true
		default:
			kind, detail, toast = "status_changed", string(beforeStatus)+" → "+string(q.Status), true
		}
	}
	updated, err := h.Store.UpdateQuest(r.Context(), q, kind, detail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v, ok := raw["tag_ids"]; ok {
		var ids []int64
		if err := json.Unmarshal(v, &ids); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid tag_ids")
			return
		}
		if err := h.Store.SetQuestTags(r.Context(), updated.ID, ids); err != nil {
			writeTagErr(w, err)
			return
		}
		updated, err = h.Store.GetQuest(r.Context(), updated.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	quiet, source := publishOpts(r)
	qid := updated.ID
	h.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &qid, Title: updated.Title, Description: updated.Description,
		Detail: detail, Toast: !quiet && toast, Source: source,
		Significance: string(updated.Significance),
		Automated:    updated.Automated,
	})
	writeJSON(w, http.StatusOK, journal.ToQuestRead(updated, timeutil.NowUTC()))
}

func (h *Handler) deleteQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.PurgeAttachments(r.Context(), ownerQuest, id)
	if err := h.Store.DeleteQuest(r.Context(), id, q.Title); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := publishOpts(r)
	h.Hub.Publish("quest_deleted", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: q.Title, Description: q.Description,
		Detail: "удалено", Toast: !quiet, Source: source,
		Automated: q.Automated,
	})
	w.WriteHeader(http.StatusNoContent)
}
