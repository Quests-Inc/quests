package journalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
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
	o, err := h.decodeEvent(r, 0)
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
	o, err := h.decodeEvent(r, id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "событие не найдено")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	saved, linked, err := h.Store.UpdateEvent(r.Context(), o)
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
	h.publishLinkedQuests(r.Context(), linked)
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
	if err := h.Store.DeleteEvent(r.Context(), id, h.DataDir); err != nil {
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

func (h *Handler) decodeEvent(r *http.Request, id int64) (journal.Event, error) {
	var body eventBody
	if id > 0 {
		current, err := h.Store.GetEvent(r.Context(), id)
		if err != nil {
			return journal.Event{}, err
		}
		body = eventBody{Title: current.Title, Description: current.Description, StartsAt: current.StartsAt.Format(time.RFC3339Nano), EndsAt: current.EndsAt.Format(time.RFC3339Nano), AllDay: current.AllDay, Color: current.Color, Icon: current.Icon}
	}
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

func eventCustomIcon(o journal.Event) string {
	if o.CustomIcon != nil {
		return *o.CustomIcon
	}
	return ""
}

func (h *Handler) getEventIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	o, err := h.Store.GetEvent(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "событие не найдено")
		return
	}
	custom := eventCustomIcon(o)
	if custom == "" || !kernel.SafeIconName(custom) {
		writeErr(w, 404, "иконка не найдена")
		return
	}
	path := filepath.Join(h.DataDir, "event-icons", custom)
	httpx.ServeIcon(w, r, path)
}

func (h *Handler) postEventIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.Store.GetEvent(r.Context(), id); errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "событие не найдено")
		return
	}
	if err := r.ParseMultipartForm(600 << 10); err != nil {
		writeErr(w, 400, "invalid multipart")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 512*1024+1))
	if err != nil || len(raw) == 0 || len(raw) > 512*1024 {
		writeErr(w, 400, "icon too large or empty")
		return
	}
	ct := hdr.Header.Get("Content-Type")
	ext := ".png"
	switch {
	case strings.Contains(ct, "jpeg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".jpg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".jpeg"):
		ext = ".jpg"
	case strings.Contains(ct, "webp"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".webp"):
		ext = ".webp"
	case strings.Contains(ct, "gif"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".gif"):
		ext = ".gif"
	case strings.Contains(ct, "svg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".svg"):
		ext = ".svg"
	}
	dir := filepath.Join(h.DataDir, "event-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := h.Store.SetEventIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	read := journal.EventToRead(saved)
	h.publishEvent(r.Context(), "event_updated", read)
	writeJSON(w, 200, read)
}

func (h *Handler) deleteEventIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	saved, err := h.Store.ClearEventIcon(r.Context(), id, h.DataDir)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "событие не найдено")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	read := journal.EventToRead(saved)
	h.publishEvent(r.Context(), "event_updated", read)
	writeJSON(w, 200, read)
}

func (h *Handler) publishEvent(ctx context.Context, kind string, o journal.EventRead) {
	h.publishSilent(ctx, kind, o.Title, map[string]any{"event_id": o.ID})
}

// publishLinkedQuests wakes journal rows whose deadline was rewritten with the event.
func (h *Handler) publishLinkedQuests(ctx context.Context, ids []int64) {
	for _, id := range ids {
		qid := id
		h.publishSilent(ctx, "quest_updated", "", map[string]any{"quest_id": qid})
	}
}
