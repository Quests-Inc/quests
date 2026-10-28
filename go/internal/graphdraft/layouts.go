package graphdraft

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/store"
)

// Saved layouts of the map: the page sends its own state as it is (the server only checks it is a JSON object of a sane size).
const (
	MaxLayouts       = 100
	MaxLayoutTitle   = 120
	MaxLayoutStateKB = 512
)

func (h *Handler) registerLayouts(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph-layouts", h.listLayouts)
	mux.HandleFunc("POST /api/graph-layouts", h.createLayout)
	mux.HandleFunc("GET /api/graph-layouts/{id}", h.getLayout)
	mux.HandleFunc("PATCH /api/graph-layouts/{id}", h.patchLayout)
	mux.HandleFunc("DELETE /api/graph-layouts/{id}", h.deleteLayout)
}

type layoutIn struct {
	Title *string         `json:"title"`
	State json.RawMessage `json:"state"`
}

func readLayout(w http.ResponseWriter, r *http.Request) (layoutIn, bool) {
	var in layoutIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxLayoutStateKB<<10+4096)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return in, false
	}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" || len([]rune(t)) > MaxLayoutTitle {
			writeErr(w, 422, "название: от 1 до 120 символов")
			return in, false
		}
		in.Title = &t
	}
	if in.State != nil {
		var obj map[string]json.RawMessage
		if len(in.State) > MaxLayoutStateKB<<10 || json.Unmarshal(in.State, &obj) != nil {
			writeErr(w, 422, "расклад: ожидается объект JSON до 512 КБ")
			return in, false
		}
	}
	return in, true
}

func layoutID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, 400, "invalid id")
		return 0, false
	}
	return id, true
}

func layoutErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "расклад не найден")
		return
	}
	writeErr(w, 500, err.Error())
}

func (h *Handler) listLayouts(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListGraphLayouts(r.Context())
	if err != nil {
		layoutErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (h *Handler) getLayout(w http.ResponseWriter, r *http.Request) {
	id, ok := layoutID(w, r)
	if !ok {
		return
	}
	l, err := h.Store.GetGraphLayout(r.Context(), id)
	if err != nil {
		layoutErr(w, err)
		return
	}
	writeJSON(w, 200, l)
}

func (h *Handler) createLayout(w http.ResponseWriter, r *http.Request) {
	in, ok := readLayout(w, r)
	if !ok {
		return
	}
	if in.Title == nil || in.State == nil {
		writeErr(w, 422, "нужны название и расклад")
		return
	}
	if n, err := h.Store.CountGraphLayouts(r.Context()); err != nil {
		layoutErr(w, err)
		return
	} else if n >= MaxLayouts {
		writeErr(w, 422, "раскладов уже 100: удалите ненужные")
		return
	}
	l, err := h.Store.CreateGraphLayout(r.Context(), *in.Title, in.State)
	if err != nil {
		layoutErr(w, err)
		return
	}
	writeJSON(w, 201, l)
}

func (h *Handler) patchLayout(w http.ResponseWriter, r *http.Request) {
	id, ok := layoutID(w, r)
	if !ok {
		return
	}
	in, ok := readLayout(w, r)
	if !ok {
		return
	}
	l, err := h.Store.UpdateGraphLayout(r.Context(), id, in.Title, in.State)
	if err != nil {
		layoutErr(w, err)
		return
	}
	writeJSON(w, 200, l)
}

func (h *Handler) deleteLayout(w http.ResponseWriter, r *http.Request) {
	id, ok := layoutID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteGraphLayout(r.Context(), id); err != nil {
		layoutErr(w, err)
		return
	}
	w.WriteHeader(204)
}
