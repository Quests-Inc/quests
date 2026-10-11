package knowledgehttp

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
)

func noteCustomIcon(n knowledge.Note) string {
	if n.CustomIcon != nil {
		return *n.CustomIcon
	}
	return ""
}

func (h *Handler) getNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	custom := noteCustomIcon(n)
	if custom == "" || !kernel.SafeIconName(custom) {
		writeErr(w, 404, "Icon not found")
		return
	}
	path := filepath.Join(h.DataDir, "note-icons", custom)
	httpx.ServeIcon(w, r, path)
}

func (h *Handler) postNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.Store.GetNote(r.Context(), id); errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
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
	dir := filepath.Join(h.DataDir, "note-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, err := h.Store.SetNoteIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, h.NotePayloadFiltered(r, updated))
}

func (h *Handler) deleteNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	updated, err := h.Store.ClearNoteIcon(r.Context(), id, h.DataDir)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, h.NotePayloadFiltered(r, updated))
}
