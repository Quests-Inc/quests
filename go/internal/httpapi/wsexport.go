package httpapi

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/wsctx"
)

// Workspace archive (quest=349): a zip with
//
//	manifest.json              format, version, workspace name, what is included
//	tables/<table>.jsonl       one JSON object per row (see store.ExportTables)
//	files/<sha256(path)[:32]>  attachment bytes, keyed by the row's webdav_path
//	icons/<kind>-<id>.<ext>    custom questline / note icons
//
// Import always creates a NEW workspace and renumbers everything.

const (
	maxImportBytes   = 2 << 30   // uploaded archive
	maxTableBytes    = 256 << 20 // one table's JSON lines, uncompressed
	maxIconBytes     = 512 << 10
	maxJSONLineBytes = 16 << 20
)

type archiveManifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	ExportedAt string `json:"exported_at"`
	Workspace  string `json:"workspace"`
	Secrets    bool   `json:"secrets"`
	Files      bool   `json:"files"`
}

func (s *Server) registerWorkspaceArchive(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces/{id}/export", s.exportWorkspace)
	mux.HandleFunc("POST /api/workspaces/import", s.importWorkspace)
}

func fileKey(webdavPath string) string {
	sum := sha256.Sum256([]byte(webdavPath))
	return hex.EncodeToString(sum[:16])
}

// exportWorkspace streams the workspace as a zip (owners only; secrets only with ?secrets=1).
func (s *Server) exportWorkspace(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	if role != store.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can export a workspace")
		return
	}
	// Export reads under the workspace being exported, not whichever the session is in.
	ctx := wsctx.With(r.Context(), id)
	name, err := s.Store.WorkspaceName(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "workspace not found")
		return
	}
	withSecrets := r.URL.Query().Get("secrets") == "1"
	withFiles := r.URL.Query().Get("files") != "0" && s.WebDAV != nil && s.WebDAV.Configured()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="quests-workspace-%d-%s.zip"`, id, time.Now().UTC().Format("20060102")))
	zw := zip.NewWriter(w)
	fail := func(err error) {
		// Headers are gone; a truncated zip (no central directory) is rejected by import.
		log.Printf("export workspace %d: %v", id, err)
	}
	mw, err := zw.Create("manifest.json")
	if err != nil {
		fail(err)
		return
	}
	_ = json.NewEncoder(mw).Encode(archiveManifest{
		Format: store.ExportFormat, Version: store.ExportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339), Workspace: name,
		Secrets: withSecrets, Files: withFiles,
	})

	paths := map[string]struct{}{}
	type icon struct {
		kind, name string
		id         int64
	}
	var icons []icon
	for _, spec := range store.ExportTables {
		if spec.Secret && !withSecrets {
			continue
		}
		tw, err := zw.Create("tables/" + spec.Name + ".jsonl")
		if err != nil {
			fail(err)
			return
		}
		enc := json.NewEncoder(tw)
		err = s.Store.ReadTable(ctx, spec, func(row map[string]any) error {
			if spec.Name == "attachment" || spec.Name == "attachment_revision" {
				if p, _ := row["webdav_path"].(string); p != "" {
					paths[p] = struct{}{}
				}
			}
			if spec.Name == "questline" || spec.Name == "note" {
				if c, _ := row["custom_icon"].(string); c != "" && store.SafeIconName(c) {
					if rid, ok := row["id"].(int64); ok {
						icons = append(icons, icon{spec.Name, c, rid})
					}
				}
			}
			return enc.Encode(row)
		})
		if err != nil {
			fail(err)
			return
		}
	}
	for _, ic := range icons {
		dir := "questline-icons"
		if ic.kind == "note" {
			dir = "note-icons"
		}
		raw, err := os.ReadFile(filepath.Join(s.DataDir, dir, ic.name))
		if err != nil {
			continue // a missing icon is cosmetic
		}
		iw, err := zw.Create("icons/" + ic.kind + "-" + strconv.FormatInt(ic.id, 10) + filepath.Ext(ic.name))
		if err != nil {
			fail(err)
			return
		}
		_, _ = iw.Write(raw)
	}
	if withFiles {
		for p := range paths {
			body, err := s.WebDAV.Get(ctx, p)
			if err != nil {
				log.Printf("export workspace %d: skip %s: %v", id, p, err)
				continue
			}
			fw, err := zw.CreateHeader(&zip.FileHeader{Name: "files/" + fileKey(p), Method: zip.Store})
			if err != nil {
				body.Close()
				fail(err)
				return
			}
			_, err = io.Copy(fw, body)
			body.Close()
			if err != nil {
				fail(err)
				return
			}
		}
	}
	if err := zw.Close(); err != nil {
		fail(err)
	}
}

// importWorkspace creates a new workspace from an uploaded archive.
func (s *Server) importWorkspace(w http.ResponseWriter, r *http.Request) {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 {
		writeErr(w, http.StatusBadRequest, "import needs an account")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid upload (or archive too large)")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "quests-import-*.zip")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		writeErr(w, http.StatusBadRequest, "could not read upload")
		return
	}
	_ = tmp.Close()
	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "это не архив пространства (zip не открывается)")
		return
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	mf := entries["manifest.json"]
	if mf == nil {
		writeErr(w, http.StatusUnprocessableEntity, "нет manifest.json — это не архив пространства")
		return
	}
	var man archiveManifest
	if rc, err := mf.Open(); err == nil {
		err = json.NewDecoder(io.LimitReader(rc, 1<<16)).Decode(&man)
		rc.Close()
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "manifest.json повреждён")
			return
		}
	}
	if man.Format != store.ExportFormat {
		writeErr(w, http.StatusUnprocessableEntity, "неизвестный формат архива")
		return
	}
	if man.Version > store.ExportVersion {
		writeErr(w, http.StatusUnprocessableEntity, fmt.Sprintf("архив версии %d новее, чем понимает этот сервер", man.Version))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(man.Workspace)
		if name == "" {
			name = "Импорт"
		}
		name += " (импорт)"
	}
	// Create the workspace, import into it; if anything fails the new workspace is dropped again.
	ws, err := s.Store.CreateWorkspace(r.Context(), p.User.ID, name, 0)
	if errors.Is(err, store.ErrWorkspaceLimit) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx := wsctx.With(r.Context(), ws.ID)
	var uploaded []string
	var importedBytes int64
	abort := func(status int, msg string) {
		for _, path := range uploaded {
			_ = s.WebDAV.Delete(context.WithoutCancel(ctx), path)
		}
		_ = s.Store.AbortWorkspace(context.WithoutCancel(r.Context()), ws.ID)
		writeErr(w, status, msg)
	}

	maxFile := s.MaxUploadBytes
	if maxFile <= 0 {
		maxFile = 25 << 20
	}
	opt := store.ImportOptions{Secrets: man.Secrets && r.FormValue("secrets") == "1"}
	if man.Files {
		opt.Attachment = func(ctx context.Context, oldPath, ownerType string, ownerID int64, filename string) (string, error) {
			f := entries["files/"+fileKey(oldPath)]
			if f == nil {
				return "", store.ErrAttachmentMissing
			}
			if f.UncompressedSize64 > uint64(maxFile) {
				return "", fmt.Errorf("вложение %q больше %d МБ", filename, maxFile>>20)
			}
			if over, used, qerr := s.overQuota(ctx, int64(f.UncompressedSize64)+importedBytes); qerr != nil {
				return "", qerr
			} else if over {
				return "", errors.New(quotaMessage(used+importedBytes, s.WorkspaceQuota))
			}
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			raw, err := io.ReadAll(io.LimitReader(rc, maxFile+1))
			rc.Close()
			if err != nil || int64(len(raw)) > maxFile {
				return "", fmt.Errorf("вложение %q: не читается или слишком большое", filename)
			}
			res, err := s.ClamAV.Scan(ctx, bytes.NewReader(raw))
			if err != nil {
				return "", fmt.Errorf("проверка на вирусы недоступна: %w", err)
			}
			if res.Infected {
				return "", fmt.Errorf("вложение %q отклонено антивирусом: %s", filename, res.Signature)
			}
			newPath := fmt.Sprintf("attachments/%s-%d/%s-%s", ownerType, ownerID, uuid.NewString(), safeFilename(filename))
			if err := s.WebDAV.Put(ctx, newPath, "application/octet-stream", bytes.NewReader(raw), int64(len(raw))); err != nil {
				return "", fmt.Errorf("загрузка на файловый сервер: %w", err)
			}
			uploaded = append(uploaded, newPath)
			importedBytes += int64(len(raw)) // rows are inserted after the callback returns: count them here
			return newPath, nil
		}
	}

	tables := func(table string, fn func(row map[string]any) error) error {
		f := entries["tables/"+table+".jsonl"]
		if f == nil {
			return nil
		}
		if f.UncompressedSize64 > maxTableBytes {
			return fmt.Errorf("таблица %s слишком большая", table)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		sc := bufio.NewScanner(io.LimitReader(rc, maxTableBytes+1))
		sc.Buffer(make([]byte, 0, 64<<10), maxJSONLineBytes)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			dec := json.NewDecoder(bytes.NewReader(line))
			dec.UseNumber()
			var row map[string]any
			if err := dec.Decode(&row); err != nil {
				return fmt.Errorf("%s: строка не разобрана: %w", table, err)
			}
			for k, v := range row {
				if n, ok := v.(json.Number); ok {
					if i, err := n.Int64(); err == nil {
						row[k] = i
					} else if f, err := n.Float64(); err == nil {
						row[k] = f
					}
				}
			}
			if err := fn(row); err != nil {
				return err
			}
		}
		return sc.Err()
	}

	rep, err := s.Store.ImportWorkspace(ctx, tables, opt)
	if err != nil {
		log.Printf("import workspace: %v", err)
		status := http.StatusUnprocessableEntity
		if errors.Is(err, context.Canceled) {
			status = 499
		}
		abort(status, "импорт не удался: "+err.Error())
		return
	}
	s.importIcons(ctx, entries, rep.IDs)
	writeJSON(w, http.StatusCreated, map[string]any{"workspace_id": ws.ID, "name": ws.Name, "report": rep})
}

// importIcons re-attaches custom icon files under the new ids.
func (s *Server) importIcons(ctx context.Context, entries map[string]*zip.File, ids map[string]map[int64]int64) {
	okExt := map[string]bool{".png": true, ".jpg": true, ".webp": true, ".gif": true, ".svg": true}
	for name, f := range entries {
		rest, found := strings.CutPrefix(name, "icons/")
		if !found || f.UncompressedSize64 > maxIconBytes {
			continue
		}
		ext := strings.ToLower(filepath.Ext(rest))
		base := strings.TrimSuffix(rest, filepath.Ext(rest))
		kind, idStr, found := strings.Cut(base, "-")
		oldID, err := strconv.ParseInt(idStr, 10, 64)
		if !found || err != nil || !okExt[ext] {
			continue
		}
		table, dir := "questline", "questline-icons"
		if kind == "note" {
			table, dir = "note", "note-icons"
		} else if kind != "questline" {
			continue
		}
		newID, ok := ids[table][oldID]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, maxIconBytes+1))
		rc.Close()
		if err != nil || len(raw) == 0 || len(raw) > maxIconBytes {
			continue
		}
		_ = os.MkdirAll(filepath.Join(s.DataDir, dir), 0o755)
		fname := strconv.FormatInt(newID, 10) + ext
		if err := os.WriteFile(filepath.Join(s.DataDir, dir, fname), raw, 0o644); err != nil {
			continue
		}
		if table == "questline" {
			_, _ = s.Store.SetQuestlineIcon(ctx, newID, fname)
		} else {
			_, _ = s.Store.SetNoteIcon(ctx, newID, fname)
		}
	}
}
