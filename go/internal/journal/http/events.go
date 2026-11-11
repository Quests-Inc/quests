package journalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListEvents(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]journal.EventRead, 0, len(rows))
	for _, o := range rows {
		out = append(out, journal.EventToRead(o))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	o, err := h.Store.GetEvent(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "событие не найдено")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, journal.EventToRead(o))
}

type eventBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	AllDay      bool   `json:"all_day"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	o, err := decodeEvent(r, 0)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	saved, err := h.Store.CreateEvent(r.Context(), o)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	read := journal.EventToRead(saved)
	h.publishEvent(r.Context(), "event_updated", read)
	writeJSON(w, http.StatusCreated, read)
}

func (h *Handler) patchEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	o, err := decodeEvent(r, id)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	saved, err := h.Store.UpdateEvent(r.Context(), o)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "событие не найдено")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	read := journal.EventToRead(saved)
	h.publishEvent(r.Context(), "event_updated", read)
	writeJSON(w, http.StatusOK, read)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	o, err := h.Store.GetEvent(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "событие не найдено")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.Store.DeleteEvent(r.Context(), id); err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "событие не найдено")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.publishEvent(r.Context(), "event_deleted", journal.EventToRead(o))
	w.WriteHeader(http.StatusNoContent)
}

func decodeEvent(r *http.Request, id int64) (journal.Event, error) {
	var body eventBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return journal.Event{}, errors.New("invalid JSON")
	}
	start, err := timeutil.ParseFlexible(strings.TrimSpace(body.StartsAt))
	if err != nil {
		return journal.Event{}, errors.New("не разобрать начало")
	}
	end, err := timeutil.ParseFlexible(strings.TrimSpace(body.EndsAt))
	if err != nil {
		return journal.Event{}, errors.New("не разобрать конец")
	}
	return journal.Event{
		ID:          id,
		Title:       body.Title,
		Description: body.Description,
		StartsAt:    start,
		EndsAt:      end,
		AllDay:      body.AllDay,
		Color:       body.Color,
		Icon:        body.Icon,
	}, nil
}

func (h *Handler) questToEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	saved, err := h.Store.QuestToEvent(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "квест не найден")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	read := journal.EventToRead(saved)
	h.publishEvent(r.Context(), "event_updated", read)
	writeJSON(w, http.StatusCreated, read)
}

func (h *Handler) publishEvent(ctx context.Context, kind string, o journal.EventRead) {
	h.publishSilent(ctx, kind, o.Title, map[string]any{"event_id": o.ID})
}
