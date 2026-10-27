package journalhttp

import (
	"encoding/json"
	"errors"
	"github.com/quests-inc/quests/go/internal/platform/trashrow"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/refs"
)

func (h *Handler) listQuestlines(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListQuestlines(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (h *Handler) getQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	row.Refs = h.ResolveRefs(r, refs.Parse(row.Title+"\n"+row.Description))
	writeJSON(w, 200, row)
}

func (h *Handler) createQuestline(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	title, _ := body["title"].(string)
	desc, _ := body["description"].(string)
	color, _ := body["color"].(string)
	icon, _ := body["icon"].(string)
	var cat *int64
	if v, ok := body["category_id"].(float64); ok {
		i := int64(v)
		cat = &i
	}
	row, err := h.Store.CreateQuestline(r.Context(), strings.TrimSpace(title), desc, cat, color, icon)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "questline_created", questlineTitle(row), map[string]any{"questline_id": row.ID})
	writeJSON(w, 201, row)
}

func (h *Handler) patchQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := h.Store.UpdateQuestline(r.Context(), id, body)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}

func (h *Handler) deleteQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	title := questlineTitle(row)
	if err := trashrow.Move(r.Context(), h.Store.DB, "questline", id); err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			writeErr(w, 404, "Questline not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "questline_deleted", title, map[string]any{"questline_id": id})
	w.WriteHeader(204)
}

func questlineTitle(row journal.QuestlineRead) string {
	return row.Title
}

func questlineCustomIcon(row journal.QuestlineRead) string {
	if row.CustomIcon != nil {
		return *row.CustomIcon
	}
	return ""
}

func (h *Handler) getQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	custom := questlineCustomIcon(row)
	if custom == "" || !kernel.SafeIconName(custom) {
		writeErr(w, 404, "Icon not found")
		return
	}
	path := filepath.Join(h.DataDir, "questline-icons", custom)
	httpx.ServeIcon(w, r, path)
}

func (h *Handler) postQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.Store.GetQuestline(r.Context(), id); errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
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
	dir := filepath.Join(h.DataDir, "questline-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	row, err := h.Store.SetQuestlineIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}

func (h *Handler) deleteQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := h.Store.ClearQuestlineIcon(r.Context(), id, h.DataDir)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}
