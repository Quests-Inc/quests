package knowledgehttp

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/valirum/quests/go/internal/knowledge"
)

// countRead feeds the reading signal (quest=353): only the web UI (a person) and MCP (an agent)
// report reads — the CLI, bots, exports and other background clients do not — and an agent
// reading a PRIVATE note, which it only sees as a placeholder, is not interest either.
func (h *Handler) countRead(r *http.Request, n knowledge.Note) {
	src := requestSource(r)
	if src == nil || (*src != "web" && *src != "mcp") {
		return
	}
	agent := *src == "mcp"
	if agent && n.IsPrivate {
		return
	}
	if _, err := h.Store.RecordNoteOpen(r.Context(), n.ID, agent); err != nil {
		log.Printf("notes: counting a read of note %d: %v", n.ID, err)
	}
}

// noteFeedback is the cheap explicit signal: this note was useful (or misleading) for the task at hand.
func (h *Handler) noteFeedback(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Useful *bool `json:"useful"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Useful == nil {
		writeErr(w, http.StatusBadRequest, `body: {"useful": true|false}`)
		return
	}
	if err := h.Store.RateNote(r.Context(), id, *body.Useful); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

// noteWeight sets the manual lever: −2 (misleading) … +2 (always worth a look).
func (h *Handler) noteWeight(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Weight *int `json:"weight"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Weight == nil || *body.Weight < -2 || *body.Weight > 2 {
		writeErr(w, http.StatusBadRequest, `body: {"weight": -2..2}`)
		return
	}
	if err := h.Store.SetNoteWeight(r.Context(), id, *body.Weight); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

// resetNoteStats forgets what was counted for the note (a mistaken streak of opens, a stale favourite).
func (h *Handler) resetNoteStats(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	if err := h.Store.ResetNoteStats(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

func (h *Handler) noteForStats(w http.ResponseWriter, r *http.Request) (int64, knowledge.Note, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return 0, n, false
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return 0, n, false
	}
	return id, n, true
}

func (h *Handler) writePopularity(w http.ResponseWriter, r *http.Request, n knowledge.Note) {
	pop, err := h.Store.Popularity(r.Context(), map[int64]time.Time{n.ID: n.CreatedAt})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, pop[n.ID])
}
