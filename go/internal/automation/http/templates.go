package automationhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/automation"
	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/refs"
)

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	var en *bool
	if v := r.URL.Query().Get("enabled"); v != "" {
		b := v == "1" || strings.EqualFold(v, "true")
		en = &b
	}
	rows, err := h.Store.ListTemplates(r.Context(), en)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.GetTemplate(r.Context(), id)
	if errors.Is(err, automation.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, row)
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := h.Store.CreateTemplate(r.Context(), body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if row.Enabled {
		h.MaterializeDue(r.Context())
	}
	writeJSON(w, 201, row)
}

func (h *Handler) patchTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := h.Store.UpdateTemplate(r.Context(), id, body)
	if errors.Is(err, automation.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if row.Enabled {
		h.MaterializeDue(r.Context())
	}
	writeJSON(w, 200, row)
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.Store.DeleteTemplate(r.Context(), id); err != nil {
		if errors.Is(err, automation.ErrNotFound) {
			writeErr(w, 404, "Template not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) copyTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.CopyTemplate(r.Context(), id)
	if errors.Is(err, automation.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, row)
}

func (h *Handler) emitTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	qid, err := h.EmitTemplate(r.Context(), id, timeutil.NowUTC())
	if errors.Is(err, h.ErrTemplateNotFound) {
		writeErr(w, http.StatusNotFound, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q, err := h.Store.Journal().GetQuest(r.Context(), qid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := journal.ToQuestRead(q, timeutil.NowUTC())
	out.Refs = h.ResolveRefs(r, refs.Parse(q.Title+"\n"+q.Description))
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) getTemplateStreak(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad template id")
		return
	}
	row, err := h.Store.TemplateStreak(r.Context(), id, timeutil.NowUTC())
	if errors.Is(err, automation.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, row)
}
