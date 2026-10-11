package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
	"github.com/valirum/quests/go/internal/webdav"
)

const (
	ownerQuest     = "quest"
	ownerQuestline = "questline"
	ownerNote      = "note"
)

func (s *Server) registerAttachments(mux *http.ServeMux) {
	for _, owner := range []struct{ seg, typ string }{
		{"quests", ownerQuest},
		{"questlines", ownerQuestline},
		{"notes", ownerNote},
	} {
		seg, typ := owner.seg, owner.typ
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
			s.listAttachments(w, r, typ)
		})
		mux.HandleFunc("POST /api/"+seg+"/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
			s.postAttachment(w, r, typ)
		})
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			s.getAttachmentContent(w, r, typ)
		})
		mux.HandleFunc("PATCH /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			s.patchAttachment(w, r, typ)
		})
		mux.HandleFunc("DELETE /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			s.deleteAttachment(w, r, typ)
		})
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments/{aid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			s.listAttachmentRevisions(w, r, typ)
		})
		mux.HandleFunc("POST /api/"+seg+"/{id}/attachments/{aid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			s.postAttachmentRevision(w, r, typ)
		})
		mux.HandleFunc("DELETE /api/"+seg+"/{id}/attachments/{aid}/revisions/{rev}", func(w http.ResponseWriter, r *http.Request) {
			s.deleteAttachmentRevision(w, r, typ)
		})
	}
	// Index of every attachment, grouped by owner. Default is metadata only
	// (no WebDAV Stat) so the journal can seed its cache in one cheap query.
	mux.HandleFunc("GET /api/attachments", s.listAllAttachments)
	// Owner-less inline view for markdown images: `![](attachment=N)`.
	mux.HandleFunc("GET /api/attachments/{aid}/image", s.getAttachmentImage)
}

func (s *Server) ownerExists(ctx context.Context, ownerType string, id int64) (bool, error) {
	if ownerType == ownerQuestline {
		return s.Store.QuestlineExists(ctx, id)
	}
	if ownerType == ownerNote {
		return s.Store.NoteExists(ctx, id)
	}
	_, err := s.Store.GetQuest(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// attachmentFor resolves {id}/{aid} and refuses an attachment that belongs to a
// different owner, so a valid id under the wrong quest is a 404, not a leak.
func (s *Server) attachmentFor(r *http.Request, ownerType string) (store.Attachment, error) {
	ownerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	aid, _ := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	a, err := s.Store.GetAttachment(r.Context(), aid)
	if err != nil {
		return store.Attachment{}, err
	}
	if a.OwnerType != ownerType || a.OwnerID != ownerID {
		return store.Attachment{}, store.ErrNotFound
	}
	return a, nil
}

// logAttachment records an attachment event against the attachment and its owner.
func (s *Server) logAttachment(ctx context.Context, kind string, a store.Attachment, detail string) {
	_ = s.Store.LogEvent(context.WithoutCancel(ctx), store.ChangeEvent{
		Kind: kind, EntityType: "attachment", EntityID: a.ID,
		ParentType: a.OwnerType, ParentID: a.OwnerID, Title: a.Filename, Detail: detail,
	})
}

func (s *Server) webdavOK() bool { return s.WebDAV != nil && s.WebDAV.Configured() }
func (s *Server) clamavOK() bool { return s.ClamAV != nil && s.ClamAV.Configured() }

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request, ownerType string) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rows, err := s.Store.ListAttachments(r.Context(), ownerType, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	probe := r.URL.Query().Get("stat") != "0"
	out := make([]AttachmentView, 0, len(rows))
	for _, a := range rows {
		out = append(out, s.decorateAttachment(r.Context(), a, probe))
	}
	writeJSON(w, 200, out)
}

// listAllAttachments is the journal cache seed: one database round-trip, grouped
// by owner. ?stat=1 opts into WebDAV probes (expensive with many files).
func (s *Server) listAllAttachments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListAllAttachments(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	probe := r.URL.Query().Get("stat") == "1"
	out := map[string]map[string][]AttachmentView{
		"quest":     {},
		"questline": {},
		"note":      {},
	}
	for _, a := range rows {
		bucket, ok := out[a.OwnerType]
		if !ok {
			continue
		}
		oid := strconv.FormatInt(a.OwnerID, 10)
		bucket[oid] = append(bucket[oid], s.decorateAttachment(r.Context(), a, probe))
	}
	writeJSON(w, 200, out)
}

// sourceUpdatedSlack absorbs the gap between the upload timestamp (taken before
// the PUT) and the file server's own mtime, which may be second-granular.
const sourceUpdatedSlack = time.Minute

// AttachmentRevisions is the version history of one attachment.
type AttachmentRevisions struct {
	AttachmentID    int64                `json:"attachment_id"`
	CurrentRevision int                  `json:"current_revision"`
	Revisions       []store.RevisionRead `json:"revisions"`
}

// AttachmentLive is what the file server says about the file right now; absent from a listing that did not ask.
type AttachmentLive struct {
	Available     bool    `json:"available"`
	SourceUpdated bool    `json:"source_updated"`
	LastModified  *string `json:"last_modified"`
}

// AttachmentView is an attachment with its live file-server facts, when they were asked for.
type AttachmentView struct {
	store.AttachmentRead `tstype:",extends"`
	*AttachmentLive      `tstype:"Partial<AttachmentLive>,extends"`
}

// decorateAttachment adds live file-server facts. available is false (and the
// download link should hide) when WebDAV is down — not an error on the listing.
// source_updated is the "актуализируй" marker: the file's Last-Modified is
// newer than the moment the current revision was uploaded, i.e. the source
// file changed on the file server after we took it.
func (s *Server) decorateAttachment(ctx context.Context, a store.Attachment, probe bool) AttachmentView {
	out := AttachmentView{AttachmentRead: store.AttachmentToRead(a)}
	if !probe {
		// Leave available/source_updated off: unknown is not "missing", and a
		// bulk index must not paint every file as unavailable before Stat.
		return out
	}
	out.AttachmentLive = &AttachmentLive{}
	if !s.webdavOK() {
		return out
	}
	info, err := s.WebDAV.Stat(ctx, a.WebDAVPath)
	if err != nil {
		return out
	}
	out.Available = true
	if !info.LastModified.IsZero() {
		out.LastModified = timeutil.ToUTCISO(&info.LastModified)
		if info.LastModified.After(a.UploadedAt.UTC().Add(sourceUpdatedSlack)) {
			out.SourceUpdated = true
		}
	}
	return out
}

func (s *Server) attachmentsForOwner(ctx context.Context, ownerType string, ownerID int64, probe bool) []AttachmentView {
	rows, err := s.Store.ListAttachments(ctx, ownerType, ownerID)
	if err != nil {
		return []AttachmentView{}
	}
	out := make([]AttachmentView, 0, len(rows))
	for _, a := range rows {
		if a.Hidden {
			continue
		}
		out = append(out, s.decorateAttachment(ctx, a, probe))
	}
	return out
}

// purgeOwnerAttachments drops metadata first, then the bytes. A leftover file
// on WebDAV is inert; a row pointing at nothing is a broken link.
func (s *Server) purgeOwnerAttachments(ctx context.Context, ownerType string, ownerID int64) {
	paths, err := s.Store.AttachmentPathsForOwner(ctx, ownerType, ownerID)
	if err != nil {
		fmt.Printf("attachments: list paths for %s-%d: %v\n", ownerType, ownerID, err)
		return
	}
	if err := s.Store.DeleteAttachmentsForOwner(ctx, ownerType, ownerID); err != nil {
		fmt.Printf("attachments: delete rows for %s-%d: %v\n", ownerType, ownerID, err)
		return
	}
	if !s.webdavOK() {
		return
	}
	for _, p := range paths {
		if err := s.WebDAV.Delete(ctx, p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachments: file left on webdav %s: %v\n", p, err)
		}
	}
}

// safeFilename keeps the original name for humans but strips anything that
// could escape the storage directory. The stored name is uuid-prefixed anyway;
// this is belt and braces.
func safeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.TrimLeft(name, ".")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\x00', '\n', '\r':
			return -1
		}
		return r
	}, name)
	if name == "" {
		name = "file"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

func (s *Server) postAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
	if !s.webdavOK() {
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	// No scanner means no upload. Storing something unscanned and calling it
	// "pending" would leave a file nobody ever goes back to check.
	if !s.clamavOK() {
		writeErr(w, 503, "virus scanning is not configured (QUESTS_CLAMAV_ADDR)")
		return
	}
	ownerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ok, err := s.ownerExists(r.Context(), ownerType, ownerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "owner not found")
		return
	}

	maxBytes := s.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, 400, "invalid multipart (or file too large)")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file required")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeErr(w, 400, "could not read upload")
		return
	}
	if len(raw) == 0 {
		writeErr(w, 400, "empty file")
		return
	}
	if int64(len(raw)) > maxBytes {
		writeErr(w, 413, fmt.Sprintf("file exceeds %d MB", maxBytes>>20))
		return
	}
	if !s.checkQuota(w, r.Context(), int64(len(raw))) {
		return
	}

	declared := strings.TrimSpace(hdr.Header.Get("Content-Type"))
	detected := mimetype.Detect(raw).String()

	res, err := s.ClamAV.Scan(r.Context(), bytes.NewReader(raw))
	if err != nil {
		// Unknown is not clean: refuse instead of storing an unscanned file.
		writeErr(w, 503, "virus scan unavailable: "+err.Error())
		return
	}
	if res.Infected {
		writeErr(w, 422, "file rejected by virus scan: "+res.Signature)
		return
	}

	filename := safeFilename(hdr.Filename)
	storedName := uuid.NewString() + "-" + filename
	webdavPath := fmt.Sprintf("attachments/%s-%d/%s", ownerType, ownerID, storedName)

	ctype := declared
	if ctype == "" {
		ctype = detected
	}
	if err := s.WebDAV.Put(r.Context(), webdavPath, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "upload to file server failed: "+err.Error())
		return
	}

	now := timeutil.NowUTC()
	a, err := s.Store.CreateAttachment(r.Context(), store.Attachment{
		OwnerType:           ownerType,
		OwnerID:             ownerID,
		Filename:            filename,
		WebDAVPath:          webdavPath,
		SizeBytes:           int64(len(raw)),
		ContentTypeDeclared: declared,
		ContentTypeDetected: detected,
		Comment:             strings.TrimSpace(r.FormValue("comment")),
		UploadedAt:          now,
		ScanStatus:          "clean",
		ScannedAt:           &now,
	})
	if err != nil {
		// Don't leave bytes behind that no row points at.
		_ = s.WebDAV.Delete(context.WithoutCancel(r.Context()), webdavPath)
		writeErr(w, 500, err.Error())
		return
	}
	out := AttachmentView{AttachmentRead: store.AttachmentToRead(a), AttachmentLive: &AttachmentLive{Available: true}}
	s.logAttachment(r.Context(), "attachment_added", a, "загружено")
	s.publishSilent(r.Context(), "attachment_created", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	writeJSON(w, 201, out)
}

func (s *Server) getAttachmentContent(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !s.webdavOK() {
		writeErr(w, 503, "attachment storage is not configured")
		return
	}
	pathToGet := a.WebDAVPath
	filename := a.Filename
	size := a.SizeBytes
	if revStr := strings.TrimSpace(r.URL.Query().Get("revision")); revStr != "" {
		revN, err := strconv.Atoi(revStr)
		if err != nil || revN < 1 {
			writeErr(w, 422, "revision must be a positive integer")
			return
		}
		rev, err := s.Store.GetAttachmentRevision(r.Context(), a.ID, revN)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		pathToGet = rev.WebDAVPath
		filename = rev.Filename
		size = rev.SizeBytes
	}
	body, err := s.WebDAV.Get(r.Context(), pathToGet)
	if errors.Is(err, webdav.ErrNotFound) {
		writeErr(w, 404, "file missing on the file server")
		return
	}
	if err != nil {
		writeErr(w, 502, "file server unreachable: "+err.Error())
		return
	}
	defer body.Close()

	// Always a download, never something the browser renders or sniffs into
	// executing. This — not an extension blocklist — is what keeps a hostile
	// file harmless on the way out.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}

// inlineImageTypes is the whole allow-list for the inline view. SVG is
// deliberately absent: it can carry script, and a raster-only rule is what
// lets the sanitizer trust `/api/attachments/N/image` as an <img> source.
var inlineImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// getAttachmentImage serves an attachment of any owner as an inline image.
// Unlike getAttachmentContent (always a download), this renders in <img>, so
// the type comes from the server-side sniff stored at upload (never from the
// client-declared one) and anything outside inlineImageTypes is refused.
func (s *Server) getAttachmentImage(w http.ResponseWriter, r *http.Request) {
	aid, _ := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	a, err := s.Store.GetAttachment(r.Context(), aid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !s.webdavOK() {
		writeErr(w, 503, "attachment storage is not configured")
		return
	}
	// Hidden cards live only on the Attachments tab: inline views 404 so a
	// markdown image falls back to the placeholder. ?preview=1 is the
	// Attachments tab's own thumbnail/preview and bypasses that.
	if a.Hidden && r.URL.Query().Get("preview") != "1" {
		writeErr(w, 404, "attachment not found")
		return
	}
	pathToGet := a.WebDAVPath
	detected := a.ContentTypeDetected
	size := a.SizeBytes
	if revStr := strings.TrimSpace(r.URL.Query().Get("revision")); revStr != "" {
		revN, err := strconv.Atoi(revStr)
		if err != nil || revN < 1 {
			writeErr(w, 422, "revision must be a positive integer")
			return
		}
		rev, err := s.Store.GetAttachmentRevision(r.Context(), a.ID, revN)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		pathToGet = rev.WebDAVPath
		detected = rev.ContentTypeDetected
		size = rev.SizeBytes
	}
	// mimetype may append parameters ("image/png; charset=…"); compare the bare type.
	ctype, _, _ := strings.Cut(detected, ";")
	ctype = strings.ToLower(strings.TrimSpace(ctype))
	if !inlineImageTypes[ctype] {
		writeErr(w, 415, "attachment is not a raster image")
		return
	}
	body, err := s.WebDAV.Get(r.Context(), pathToGet)
	if errors.Is(err, webdav.ErrNotFound) {
		writeErr(w, 404, "file missing on the file server")
		return
	}
	if err != nil {
		writeErr(w, 502, "file server unreachable: "+err.Error())
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Even opened directly in a tab the response can't run anything.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=300")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}

// contentDisposition sends an ASCII fallback plus RFC 5987 UTF-8, so a Russian
// filename survives without breaking older clients.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		ascii, urlEscapePath(name))
}

func urlEscapePath(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func (s *Server) patchAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var body struct {
		Comment         *string `json:"comment"`
		CurrentRevision *int    `json:"current_revision"`
		Hidden          *bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	updated := a
	if body.CurrentRevision != nil {
		updated, err = s.Store.SetCurrentAttachmentRevision(r.Context(), a.ID, *body.CurrentRevision)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.Comment != nil {
		updated, err = s.Store.SetAttachmentComment(r.Context(), a.ID, strings.TrimSpace(*body.Comment))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.Hidden != nil {
		updated, err = s.Store.SetAttachmentHidden(r.Context(), a.ID, *body.Hidden)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.CurrentRevision != nil {
		s.logAttachment(r.Context(), "attachment_version", updated, fmt.Sprintf("текущая версия v%d", updated.CurrentRevision))
	}
	if body.Hidden != nil && *body.Hidden != a.Hidden {
		if *body.Hidden {
			s.logAttachment(r.Context(), "attachment_hidden", updated, "скрыто")
		} else {
			s.logAttachment(r.Context(), "attachment_hidden", updated, "возвращено")
		}
	}
	if body.Comment == nil && body.CurrentRevision == nil && body.Hidden == nil {
		writeJSON(w, 200, store.AttachmentToRead(a))
		return
	}
	s.publishSilent(r.Context(), "attachment_updated", updated.Filename, map[string]any{
		"attachment_id": updated.ID,
		"owner_type":    updated.OwnerType,
		"owner_id":      updated.OwnerID,
	})
	writeJSON(w, 200, store.AttachmentToRead(updated))
}

func (s *Server) listAttachmentRevisions(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	revs, err := s.Store.ListAttachmentRevisions(r.Context(), a.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]store.RevisionRead, 0, len(revs))
	for _, rev := range revs {
		out = append(out, store.RevisionToRead(rev, a.CurrentRevision))
	}
	writeJSON(w, 200, AttachmentRevisions{AttachmentID: a.ID, CurrentRevision: a.CurrentRevision, Revisions: out})
}

func (s *Server) postAttachmentRevision(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !s.webdavOK() {
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	if !s.clamavOK() {
		writeErr(w, 503, "virus scanning is not configured (QUESTS_CLAMAV_ADDR)")
		return
	}

	maxBytes := s.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, 400, "invalid multipart (or file too large)")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file required")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeErr(w, 400, "could not read upload")
		return
	}
	if len(raw) == 0 {
		writeErr(w, 400, "empty file")
		return
	}
	if int64(len(raw)) > maxBytes {
		writeErr(w, 413, fmt.Sprintf("file exceeds %d MB", maxBytes>>20))
		return
	}
	if !s.checkQuota(w, r.Context(), int64(len(raw))) {
		return
	}

	declared := strings.TrimSpace(hdr.Header.Get("Content-Type"))
	detected := mimetype.Detect(raw).String()
	res, err := s.ClamAV.Scan(r.Context(), bytes.NewReader(raw))
	if err != nil {
		writeErr(w, 503, "virus scan unavailable: "+err.Error())
		return
	}
	if res.Infected {
		writeErr(w, 422, "file rejected by virus scan: "+res.Signature)
		return
	}

	filename := safeFilename(hdr.Filename)
	if filename == "file" && a.Filename != "" {
		// Keep the card name when the client sends a generic/temp path.
		filename = a.Filename
	}
	storedName := uuid.NewString() + "-" + filename
	webdavPath := fmt.Sprintf("attachments/%s-%d/%s", a.OwnerType, a.OwnerID, storedName)
	ctype := declared
	if ctype == "" {
		ctype = detected
	}
	if err := s.WebDAV.Put(r.Context(), webdavPath, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "upload to file server failed: "+err.Error())
		return
	}

	now := timeutil.NowUTC()
	comment := strings.TrimSpace(r.FormValue("comment"))
	if comment == "" {
		comment = a.Comment
	}
	updated, dropPaths, err := s.Store.AddAttachmentRevision(r.Context(), a.ID, store.AttachmentRevision{
		Filename:            filename,
		WebDAVPath:          webdavPath,
		SizeBytes:           int64(len(raw)),
		ContentTypeDeclared: declared,
		ContentTypeDetected: detected,
		Comment:             comment,
		UploadedAt:          now,
		ScanStatus:          "clean",
		ScannedAt:           &now,
	})
	if err != nil {
		_ = s.WebDAV.Delete(context.WithoutCancel(r.Context()), webdavPath)
		writeErr(w, 500, err.Error())
		return
	}
	for _, p := range dropPaths {
		if err := s.WebDAV.Delete(context.WithoutCancel(r.Context()), p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachment %d: rotated file left on webdav: %v\n", a.ID, err)
		}
	}
	out := AttachmentView{AttachmentRead: store.AttachmentToRead(updated), AttachmentLive: &AttachmentLive{Available: true}}
	s.logAttachment(r.Context(), "attachment_version", updated, fmt.Sprintf("новая версия v%d", updated.CurrentRevision))
	s.publishSilent(r.Context(), "attachment_updated", updated.Filename, map[string]any{
		"attachment_id": updated.ID,
		"owner_type":    updated.OwnerType,
		"owner_id":      updated.OwnerID,
		"revision":      updated.CurrentRevision,
	})
	writeJSON(w, 201, out)
}

func (s *Server) deleteAttachmentRevision(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	revN, err := strconv.Atoi(r.PathValue("rev"))
	if err != nil || revN < 1 {
		writeErr(w, 422, "revision must be a positive integer")
		return
	}
	davPath, err := s.Store.DeleteAttachmentRevision(r.Context(), a.ID, revN)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, 422, "cannot delete the current revision")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "revision not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if s.webdavOK() {
		if err := s.WebDAV.Delete(r.Context(), davPath); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachment %d rev %d: file left on webdav: %v\n", a.ID, revN, err)
		}
	}
	s.logAttachment(r.Context(), "attachment_version", a, fmt.Sprintf("удалена версия v%d", revN))
	s.publishSilent(r.Context(), "attachment_updated", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	w.WriteHeader(204)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := s.attachmentFor(r, ownerType)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	revs, err := s.Store.ListAttachmentRevisions(r.Context(), a.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	paths := make([]string, 0, len(revs)+1)
	seen := map[string]bool{}
	for _, rev := range revs {
		if !seen[rev.WebDAVPath] {
			seen[rev.WebDAVPath] = true
			paths = append(paths, rev.WebDAVPath)
		}
	}
	if a.WebDAVPath != "" && !seen[a.WebDAVPath] {
		paths = append(paths, a.WebDAVPath)
	}
	// Drop the row first: a leftover file on WebDAV is inert, a row pointing at
	// nothing is a broken link in the UI.
	if err := s.Store.DeleteAttachment(r.Context(), a.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if s.webdavOK() {
		for _, p := range paths {
			if err := s.WebDAV.Delete(r.Context(), p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
				fmt.Printf("attachment %d: file left on webdav: %v\n", a.ID, err)
			}
		}
	}
	s.logAttachment(r.Context(), "attachment_deleted", a, "удалено")
	s.publishSilent(r.Context(), "attachment_deleted", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	w.WriteHeader(204)
}
