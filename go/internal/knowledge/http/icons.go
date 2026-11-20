package knowledgehttp

import (
	"encoding/json"
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
	ext := iconExt(ct, hdr.Filename)
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

func (h *Handler) postNoteIcons(w http.ResponseWriter, r *http.Request) {
	batch, err := readNoteIconBatch(r)
	if err != nil {
		var be *knowledge.IconBatchError
		if errors.As(err, &be) {
			writeErr(w, be.Code, be.Msg)
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	updated, err := h.Store.ApplyNoteIcons(r.Context(), h.DataDir, batch)
	if err != nil {
		var be *knowledge.IconBatchError
		if errors.As(err, &be) {
			writeErr(w, be.Code, be.Msg)
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]any, 0, len(updated))
	for _, n := range updated {
		h.logNote(r.Context(), "note_updated", n, "изменена", 10*time.Minute)
		out = append(out, h.NotePayloadFiltered(r, n))
	}
	writeJSON(w, 200, map[string]any{"notes": out})
}

func readNoteIconBatch(r *http.Request) (knowledge.NoteIconBatch, error) {
	var batch knowledge.NoteIconBatch
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(600 << 10); err != nil {
			return batch, errors.New("invalid multipart")
		}
		ids, err := parseNoteIDs(r.FormValue("ids"))
		if err != nil {
			return batch, err
		}
		batch.IDs = ids
		batch.Color = strings.TrimSpace(r.FormValue("color"))
		batch.Icon = strings.TrimSpace(r.FormValue("icon"))
		if v := strings.TrimSpace(r.FormValue("copy_from")); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				return batch, &knowledge.IconBatchError{Code: 422, Msg: "copy_from must be a note id"}
			}
			batch.CopyFrom = id
		}
		file, hdr, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			raw, err := io.ReadAll(io.LimitReader(file, 512*1024+1))
			if err != nil || len(raw) == 0 || len(raw) > 512*1024 {
				return batch, &knowledge.IconBatchError{Code: 422, Msg: "icon too large or empty"}
			}
			batch.File = raw
			batch.Ext = iconExt(hdr.Header.Get("Content-Type"), hdr.Filename)
		}
		return batch, nil
	}
	var body struct {
		IDs      []int64 `json:"ids"`
		Color    string  `json:"color"`
		Icon     string  `json:"icon"`
		CopyFrom int64   `json:"copy_from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return batch, errors.New("invalid JSON")
	}
	batch.IDs = body.IDs
	batch.Color = strings.TrimSpace(body.Color)
	batch.Icon = strings.TrimSpace(body.Icon)
	batch.CopyFrom = body.CopyFrom
	return batch, nil
}

func parseNoteIDs(raw string) ([]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &knowledge.IconBatchError{Code: 422, Msg: "ids required"}
	}
	var ids []int64
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			return nil, &knowledge.IconBatchError{Code: 422, Msg: "ids must be unique positive note ids"}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func iconExt(ct, filename string) string {
	name := strings.ToLower(filename)
	switch {
	case strings.Contains(ct, "jpeg"), strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return ".jpg"
	case strings.Contains(ct, "webp"), strings.HasSuffix(name, ".webp"):
		return ".webp"
	case strings.Contains(ct, "gif"), strings.HasSuffix(name, ".gif"):
		return ".gif"
	case strings.Contains(ct, "svg"), strings.HasSuffix(name, ".svg"):
		return ".svg"
	default:
		return ".png"
	}
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
