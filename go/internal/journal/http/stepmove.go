package journalhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// postStepMove moves a step to another quest (end of its steps). A quest keeps
// at least one step, like deleting does.
func (h *Handler) postStepMove(w http.ResponseWriter, r *http.Request) {
	fromID, err1 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	stepID, err2 := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	if err1 != nil || err2 != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		QuestID int64 `json:"quest_id"`
		// AllowEmpty lets a quest end up with no steps for now; the draft
		// commit sets it after checking the final state of every quest.
		AllowEmpty bool `json:"allow_empty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.QuestID <= 0 {
		writeErr(w, 400, "quest_id required")
		return
	}
	if in.QuestID == fromID {
		writeErr(w, 422, "шаг уже в этом квесте")
		return
	}
	from, err := h.Store.GetQuest(r.Context(), fromID)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	to, err := h.Store.GetQuest(r.Context(), in.QuestID)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Target quest not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(from.Steps) <= 1 && !in.AllowEmpty {
		writeErr(w, 422, "нельзя унести последний шаг квеста")
		return
	}
	var moved journal.Step
	var rest []journal.Step
	for _, st := range from.Steps {
		if st.ID == stepID {
			moved = st
			continue
		}
		rest = append(rest, st)
	}
	if moved.ID == 0 {
		writeErr(w, 404, "Step not found")
		return
	}
	order := 0
	for _, st := range to.Steps {
		if st.SortOrder >= order {
			order = st.SortOrder + 1
		}
	}
	moved.QuestID, moved.SortOrder, moved.WaitPrevious = to.ID, order, false
	now := timeutil.NowUTC()
	from.Steps = rest
	to.Steps = append(to.Steps, moved)
	journal.SyncStatusFromSteps(&from, now)
	journal.SyncStatusFromSteps(&to, now)
	from.UpdatedAt, to.UpdatedAt = now, now
	if err := h.Store.MoveStep(r.Context(), stepID, from, to, order); err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			writeErr(w, 404, "Step not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	for _, q := range []journal.Quest{from, to} {
		id := q.ID
		h.Hub.Publish("quest_updated", events.PublishOpts{
			Workspace: wsctx.From(r.Context()),
			QuestID:   &id, Title: q.Title, Detail: "перенос шага", Source: "web", Significance: string(q.Significance),
		})
	}
	nf, _ := h.Store.GetQuest(r.Context(), fromID)
	nt, _ := h.Store.GetQuest(r.Context(), in.QuestID)
	writeJSON(w, 200, map[string]any{
		"from": journal.ToQuestRead(nf, timeutil.NowUTC()),
		"to":   journal.ToQuestRead(nt, timeutil.NowUTC()),
	})
}
