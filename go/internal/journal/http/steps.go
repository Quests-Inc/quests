package journalhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// StepView is one step with a brief of its quest (GET /api/steps/{id}).
type StepView struct {
	Step  journal.StepRead `json:"step"`
	Quest StepQuestBrief   `json:"quest"`
}

// StepQuestBrief is the part of a quest worth showing next to one of its steps.
type StepQuestBrief struct {
	ID            int64               `json:"id"`
	Title         string              `json:"title"`
	Status        journal.QuestStatus `json:"status"`
	ProgressLabel string              `json:"progress_label"`
}

// getStep answers for a step by its id alone, without loading anything but its own quest.
func (h *Handler) getStep(w http.ResponseWriter, r *http.Request) {
	sid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	qid, err := h.Store.QuestIDForStep(r.Context(), sid)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q, err := h.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	read := journal.ToQuestRead(q, timeutil.NowUTC())
	for _, st := range read.Steps {
		if st.ID == sid {
			writeJSON(w, http.StatusOK, StepView{
				Step:  st,
				Quest: StepQuestBrief{ID: read.ID, Title: read.Title, Status: read.Status, ProgressLabel: read.ProgressLabel},
			})
			return
		}
	}
	writeErr(w, http.StatusNotFound, "Step not found")
}

func (h *Handler) addStep(w http.ResponseWriter, r *http.Request) {
	qid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body journal.StepCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ord := 0
	if body.SortOrder != nil {
		ord = *body.SortOrder
	} else if len(q.Steps) > 0 {
		for _, st := range q.Steps {
			if st.SortOrder >= ord {
				ord = st.SortOrder + 1
			}
		}
	}
	cmd, iv := journal.NormalizeCheck(body.CheckCommand, body.CheckIntervalSeconds)
	st := journal.Step{
		QuestID: qid, Title: strings.TrimSpace(body.Title), Description: body.Description,
		ProgressCurrent: body.ProgressCurrent, ProgressTotal: body.ProgressTotal,
		SortOrder: ord, CheckCommand: cmd, CheckIntervalSeconds: iv,
		RunOn: kernel.NormalizeRunOn(body.RunOn),
	}
	if !kernel.ValidRunOn(st.RunOn) {
		writeErr(w, http.StatusUnprocessableEntity, "run_on: "+kernel.ErrBadRunOn.Error())
		return
	}
	if body.WaitPrevious != nil {
		st.WaitPrevious = *body.WaitPrevious
	}
	if body.RunMode != nil {
		st.RunMode = journal.NormalizeRunMode(*body.RunMode)
	}
	journal.NormalizeStepCheck(&st)
	if st.Title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "title required")
		return
	}
	if st.ProgressTotal < 1 {
		st.ProgressTotal = 1
	}
	q.Steps = append(q.Steps, st)
	now := timeutil.NowUTC()
	journal.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	updated, err := h.Store.AddStep(r.Context(), qid, st, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := publishOpts(r)
	id := updated.ID
	_ = quiet
	h.Hub.Publish("quest_updated", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: "+шаги: 1",
		Toast: false, Source: source, Significance: string(updated.Significance),
		Automated: updated.Automated,
	})
	writeJSON(w, http.StatusCreated, journal.ToQuestRead(updated, timeutil.NowUTC()))
}

func (h *Handler) patchStep(w http.ResponseWriter, r *http.Request) {
	qid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sid, _ := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	var body journal.StepUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var st *journal.Step
	for i := range q.Steps {
		if q.Steps[i].ID == sid {
			st = &q.Steps[i]
			break
		}
	}
	if st == nil {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	wasDone := st.Done
	if body.Title != nil {
		st.Title = *body.Title
	}
	if body.Description != nil {
		st.Description = *body.Description
	}
	if body.ProgressCurrent != nil {
		st.ProgressCurrent = *body.ProgressCurrent
	}
	if body.ProgressTotal != nil {
		st.ProgressTotal = *body.ProgressTotal
	}
	if body.SortOrder != nil {
		st.SortOrder = *body.SortOrder
	}
	if st.ScriptID != nil && (body.CheckCommand == nil || strings.TrimSpace(*body.CheckCommand) == "") {
		// A step that runs a script has no command to normalise; its interval is its own.
		if body.CheckIntervalSeconds != nil {
			v := max(*body.CheckIntervalSeconds, 15)
			st.CheckIntervalSeconds = &v
		}
	} else if body.CheckCommand != nil || body.CheckIntervalSeconds != nil {
		cmd := st.CheckCommand
		iv := st.CheckIntervalSeconds
		if body.CheckCommand != nil {
			cmd = body.CheckCommand
		}
		if body.CheckIntervalSeconds != nil {
			iv = body.CheckIntervalSeconds
		}
		st.CheckCommand, st.CheckIntervalSeconds = journal.NormalizeCheck(cmd, iv)
	}
	if body.WaitPrevious != nil {
		st.WaitPrevious = *body.WaitPrevious
	}
	if body.RunMode != nil {
		st.RunMode = journal.NormalizeRunMode(*body.RunMode)
	}
	if body.RunOn != nil {
		st.RunOn = kernel.NormalizeRunOn(body.RunOn) // "" clears
		if !kernel.ValidRunOn(st.RunOn) {
			writeErr(w, http.StatusUnprocessableEntity, "run_on: "+kernel.ErrBadRunOn.Error())
			return
		}
	}
	journal.NormalizeStepCheck(st)
	journal.ClampStep(st)
	now := timeutil.NowUTC()
	journal.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	kind := "quest_updated"
	detail := "изменено"
	if st.Done {
		kind = "step_completed"
		detail = st.Title
	} else if wasDone {
		kind = "step_reopened"
		detail = st.Title
	}
	updated, err := h.Store.UpdateStep(r.Context(), *st, q, kind, detail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := publishOpts(r)
	id := updated.ID
	toast := kind == "step_completed" || kind == "quest_completed"
	h.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: detail,
		Toast: !quiet && toast, Source: source, Significance: string(updated.Significance),
		StepTitle: st.Title, Automated: updated.Automated,
	})
	writeJSON(w, http.StatusOK, journal.ToQuestRead(updated, timeutil.NowUTC()))
}

func (h *Handler) deleteStep(w http.ResponseWriter, r *http.Request) {
	qid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sid, _ := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	q, err := h.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(q.Steps) <= 1 {
		writeErr(w, http.StatusBadRequest, "Cannot delete the last step")
		return
	}
	found := false
	var next []journal.Step
	for _, st := range q.Steps {
		if st.ID == sid {
			found = true
			continue
		}
		next = append(next, st)
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	q.Steps = next
	now := timeutil.NowUTC()
	journal.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	updated, err := h.Store.DeleteStep(r.Context(), qid, sid, q)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := publishOpts(r)
	id := updated.ID
	_ = quiet
	h.Hub.Publish("quest_updated", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: "−шаги: 1",
		Toast: false, Source: source, Significance: string(updated.Significance),
		Automated: updated.Automated,
	})
	writeJSON(w, http.StatusOK, journal.ToQuestRead(updated, timeutil.NowUTC()))
}
