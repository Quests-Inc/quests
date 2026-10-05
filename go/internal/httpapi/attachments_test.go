package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valirum/quests/go/internal/clamav"
	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
	"github.com/valirum/quests/go/internal/webdav"
)

const attachmentsSchema = `
CREATE TABLE questcategory (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL,
	label TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	created_at DATETIME NOT NULL
);
CREATE TABLE questline (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	category_id INTEGER,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	icon TEXT NOT NULL DEFAULT 'document',
	custom_icon TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE TABLE quest (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	significance TEXT NOT NULL,
	pinned INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	deadline_at DATETIME,
	duration_seconds INTEGER,
	category_id INTEGER,
	questline_id INTEGER,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	completed_at DATETIME,
	template_id INTEGER,
	period_key TEXT,
	automated INTEGER NOT NULL DEFAULT 0,
	source TEXT
);
CREATE TABLE queststep (
	id INTEGER PRIMARY KEY,
	quest_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	progress_current INTEGER NOT NULL DEFAULT 0,
	progress_total INTEGER NOT NULL DEFAULT 1,
	sort_order INTEGER NOT NULL DEFAULT 0,
	check_command TEXT,
	check_interval_seconds INTEGER,
	check_last_run_at DATETIME,
	wait_previous INTEGER NOT NULL DEFAULT 0,
	run_mode TEXT NOT NULL DEFAULT 'poll',
	run_status TEXT
);
CREATE TABLE questchangelog (
	id INTEGER PRIMARY KEY,
	at DATETIME NOT NULL,
	kind TEXT NOT NULL,
	quest_id INTEGER,
	title TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	significance TEXT,
	revision INTEGER,
	comment TEXT,
	entity_type TEXT NOT NULL DEFAULT 'quest',
	entity_id INTEGER,
	parent_type TEXT,
	parent_id INTEGER
);
CREATE TABLE questtemplate (
	id INTEGER PRIMARY KEY,
	questline_id INTEGER
);
CREATE TABLE attachment (
	id INTEGER PRIMARY KEY,
	owner_type TEXT NOT NULL,
	owner_id INTEGER NOT NULL,
	filename TEXT NOT NULL,
	webdav_path TEXT NOT NULL UNIQUE,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	content_type_declared TEXT NOT NULL DEFAULT '',
	content_type_detected TEXT NOT NULL DEFAULT '',
	comment TEXT NOT NULL DEFAULT '',
	uploaded_at DATETIME NOT NULL,
	scan_status TEXT NOT NULL DEFAULT 'pending',
	scanned_at DATETIME,
	current_revision INTEGER NOT NULL DEFAULT 1,
	hidden INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE attachment_revision (
	id INTEGER PRIMARY KEY,
	attachment_id INTEGER NOT NULL,
	revision INTEGER NOT NULL,
	filename TEXT NOT NULL,
	webdav_path TEXT NOT NULL UNIQUE,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	content_type_declared TEXT NOT NULL DEFAULT '',
	content_type_detected TEXT NOT NULL DEFAULT '',
	comment TEXT NOT NULL DEFAULT '',
	uploaded_at DATETIME NOT NULL,
	scan_status TEXT NOT NULL DEFAULT 'pending',
	scanned_at DATETIME,
	UNIQUE(attachment_id, revision)
);
CREATE TABLE note (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	pinned INTEGER NOT NULL DEFAULT 0,
	is_readme INTEGER NOT NULL DEFAULT 0,
	is_private INTEGER NOT NULL DEFAULT 0,
	is_category INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	parent_id INTEGER,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	icon TEXT NOT NULL DEFAULT 'document',
	custom_icon TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE TABLE tag (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	label TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	created_at DATETIME NOT NULL
);
CREATE TABLE quest_tag (
	quest_id INTEGER NOT NULL,
	tag_id INTEGER NOT NULL,
	PRIMARY KEY (quest_id, tag_id)
);
CREATE TABLE template_tag (
	template_id INTEGER NOT NULL,
	tag_id INTEGER NOT NULL,
	PRIMARY KEY (template_id, tag_id)
);
`

type davFile struct {
	body  []byte
	mod   time.Time
	ctype string
}

type fakeDAV struct {
	mu    sync.Mutex
	files map[string]davFile
}

func newFakeDAV() *httptest.Server {
	fs := &fakeDAV{files: map[string]davFile{}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Path, "/")
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			fs.mu.Lock()
			fs.files[path] = davFile{
				body:  body,
				mod:   time.Now().UTC(),
				ctype: r.Header.Get("Content-Type"),
			}
			fs.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			fs.mu.Lock()
			f, ok := fs.files[path]
			fs.mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Last-Modified", f.mod.Format(http.TimeFormat))
			_, _ = w.Write(f.body)
		case http.MethodHead:
			fs.mu.Lock()
			f, ok := fs.files[path]
			fs.mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Last-Modified", f.mod.Format(http.TimeFormat))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(f.body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			fs.mu.Lock()
			_, ok := fs.files[path]
			delete(fs.files, path)
			fs.mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

// fakeClamd is the server half of INSTREAM, copied in spirit from clamav_test
// so httpapi tests don't import an internal test helper.
func fakeClamd(t *testing.T, reply string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1)
				for {
					n, err := c.Read(buf)
					if n > 0 && buf[0] == 0 {
						break
					}
					if err != nil {
						return
					}
				}
				var hdr [4]byte
				for {
					if _, err := io.ReadFull(c, hdr[:]); err != nil {
						return
					}
					size := binary.BigEndian.Uint32(hdr[:])
					if size == 0 {
						break
					}
					if _, err := io.CopyN(io.Discard, c, int64(size)); err != nil {
						return
					}
				}
				_, _ = c.Write([]byte(reply + "\x00"))
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func newAttachmentServer(t *testing.T, davURL, clamAddr string, maxUpload int64) *Server {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(attachmentsSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	st := &store.Store{DB: db}
	if maxUpload <= 0 {
		maxUpload = 25 << 20
	}
	s := &Server{
		Store:          st,
		Hub:            events.New(),
		AuthRequired:   false,
		WebDAV:         webdav.New(davURL, "", ""),
		ClamAV:         clamav.New(clamAddr),
		MaxUploadBytes: maxUpload,
	}
	return s
}

func seedQuest(t *testing.T, s *Server) int64 {
	t.Helper()
	now := timeutil.NowUTC()
	q, err := s.Store.CreateQuest(t.Context(), domain.Quest{
		Title:        "with files",
		Status:       domain.StatusActive,
		Significance: domain.SigCommon,
		CreatedAt:    now,
		UpdatedAt:    now,
		Steps:        []domain.Step{{Title: "one", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatalf("create quest: %v", err)
	}
	return q.ID
}

func postFile(t *testing.T, h http.Handler, path, filename, comment string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if comment != "" {
		_ = mw.WriteField("comment", comment)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAttachmentHappyPath(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	qid := seedQuest(t, s)
	h := s.Handler()

	w := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "note.txt", "todo source", []byte("hello files"))
	if w.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["filename"] != "note.txt" {
		t.Fatalf("filename = %v", created["filename"])
	}
	if created["comment"] != "todo source" {
		t.Fatalf("comment = %v", created["comment"])
	}
	if created["scan_status"] != "clean" {
		t.Fatalf("scan_status = %v", created["scan_status"])
	}
	if created["available"] != true {
		t.Fatalf("available = %v, want true after upload", created["available"])
	}
	aid := int64(created["id"].(float64))

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments", qid), nil))
	if list.Code != 200 {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("list len = %d", len(rows))
	}
	if rows[0]["available"] != true {
		t.Fatalf("listed available = %v", rows[0]["available"])
	}

	dl := httptest.NewRecorder()
	h.ServeHTTP(dl, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), nil))
	if dl.Code != 200 {
		t.Fatalf("download = %d %s", dl.Code, dl.Body.String())
	}
	if got := dl.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if dl.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff")
	}
	if dl.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", dl.Header().Get("Content-Type"))
	}
	if dl.Body.String() != "hello files" {
		t.Fatalf("body = %q", dl.Body.String())
	}

	patch := doJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid),
		map[string]string{"comment": "updated note"}, nil)
	if patch.Code != 200 {
		t.Fatalf("patch = %d %s", patch.Code, patch.Body.String())
	}

	del := httptest.NewRecorder()
	h.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), nil))
	if del.Code != 204 {
		t.Fatalf("delete = %d %s", del.Code, del.Body.String())
	}
}

func TestAttachmentRevisions(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	h := s.Handler()
	qid := seedQuest(t, s)

	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "doc.txt", "v1", []byte("one"))
	if up.Code != 201 {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(up.Body.Bytes(), &created)
	aid := int64(created["id"].(float64))
	if created["revision"] != float64(1) || created["revision_count"] != float64(1) {
		t.Fatalf("rev fields = %v", created)
	}

	v2 := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments/%d/revisions", qid, aid), "doc.txt", "v2", []byte("two"))
	if v2.Code != 201 {
		t.Fatalf("rev2 = %d %s", v2.Code, v2.Body.String())
	}
	var cur map[string]any
	_ = json.Unmarshal(v2.Body.Bytes(), &cur)
	if cur["revision"] != float64(2) || cur["revision_count"] != float64(2) {
		t.Fatalf("after v2 = %v", cur)
	}

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d/revisions", qid, aid), nil))
	if list.Code != 200 {
		t.Fatalf("list revs = %d %s", list.Code, list.Body.String())
	}
	var hist map[string]any
	_ = json.Unmarshal(list.Body.Bytes(), &hist)
	revs, _ := hist["revisions"].([]any)
	if len(revs) != 2 {
		t.Fatalf("revisions=%v", hist)
	}

	// Download old revision
	old := httptest.NewRecorder()
	h.ServeHTTP(old, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d?revision=1", qid, aid), nil))
	if old.Code != 200 || old.Body.String() != "one" {
		t.Fatalf("old body = %d %q", old.Code, old.Body.String())
	}
	curDL := httptest.NewRecorder()
	h.ServeHTTP(curDL, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), nil))
	if curDL.Code != 200 || curDL.Body.String() != "two" {
		t.Fatalf("current body = %d %q", curDL.Code, curDL.Body.String())
	}

	// Switch current back to v1
	raw, _ := json.Marshal(map[string]any{"current_revision": 1})
	sw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(sw, req)
	if sw.Code != 200 {
		t.Fatalf("switch = %d %s", sw.Code, sw.Body.String())
	}
	curDL = httptest.NewRecorder()
	h.ServeHTTP(curDL, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), nil))
	if curDL.Body.String() != "one" {
		t.Fatalf("after switch = %q", curDL.Body.String())
	}

	// Cannot delete current
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/quests/%d/attachments/%d/revisions/1", qid, aid), nil))
	if bad.Code != 422 {
		t.Fatalf("delete current = %d", bad.Code)
	}
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/quests/%d/attachments/%d/revisions/2", qid, aid), nil))
	if ok.Code != 204 {
		t.Fatalf("delete old = %d %s", ok.Code, ok.Body.String())
	}
}

func TestAttachmentInfectedRejected(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: Eicar-Test-Signature FOUND")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	qid := seedQuest(t, s)
	w := postFile(t, s.Handler(), fmt.Sprintf("/api/quests/%d/attachments", qid), "eicar.txt", "", []byte("X5O!"))
	if w.Code != 422 {
		t.Fatalf("status = %d want 422 body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Eicar-Test-Signature") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAttachmentScannerDownIs503(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "INSTREAM size limit exceeded. ERROR")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	qid := seedQuest(t, s)
	w := postFile(t, s.Handler(), fmt.Sprintf("/api/quests/%d/attachments", qid), "a.txt", "", []byte("ok"))
	if w.Code != 503 {
		t.Fatalf("status = %d want 503 body %s", w.Code, w.Body.String())
	}
}

func TestAttachmentTooLargeIs413(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 8)
	qid := seedQuest(t, s)
	w := postFile(t, s.Handler(), fmt.Sprintf("/api/quests/%d/attachments", qid), "big.bin", "", bytes.Repeat([]byte("x"), 32))
	if w.Code != 413 {
		t.Fatalf("status = %d want 413 body %s", w.Code, w.Body.String())
	}
}

func TestAttachmentWrongOwnerIs404(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	h := s.Handler()
	a := seedQuest(t, s)
	b := seedQuest(t, s)
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", a), "x.txt", "", []byte("x"))
	if up.Code != 201 {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(up.Body.Bytes(), &created)
	aid := int64(created["id"].(float64))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d", b, aid), nil))
	if w.Code != 404 {
		t.Fatalf("status = %d want 404 body %s", w.Code, w.Body.String())
	}
}

func TestAttachmentDeleteQuestPurgesFiles(t *testing.T) {
	davSrv := newFakeDAV()
	t.Cleanup(davSrv.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, davSrv.URL, clam, 0)
	h := s.Handler()
	qid := seedQuest(t, s)
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "keep.txt", "", []byte("bye"))
	if up.Code != 201 {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}

	del := httptest.NewRecorder()
	h.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/quests/%d", qid), nil))
	if del.Code != 204 {
		t.Fatalf("delete quest = %d %s", del.Code, del.Body.String())
	}
	rows, err := s.Store.ListAttachments(t.Context(), ownerQuest, qid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows left = %d", len(rows))
	}
}

func TestAttachmentUnconfiguredIs503(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	qid := seedQuest(t, s)
	w := postFile(t, s.Handler(), fmt.Sprintf("/api/quests/%d/attachments", qid), "a.txt", "", []byte("x"))
	if w.Code != 503 {
		t.Fatalf("status = %d want 503 body %s", w.Code, w.Body.String())
	}
}

func TestAttachmentSourceUpdatedMarker(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	qid := seedQuest(t, s)
	h := s.Handler()
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "src.md", "", []byte("# todo"))
	if up.Code != 201 {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}
	listRows := func() []map[string]any {
		list := httptest.NewRecorder()
		h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments", qid), nil))
		var rows []map[string]any
		if err := json.Unmarshal(list.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	// Fresh upload (even after the quest was last edited) is not "source updated".
	if rows := listRows(); len(rows) != 1 || rows[0]["source_updated"] != false {
		t.Fatalf("fresh upload flagged: %v", rows)
	}
	// File Last-Modified is "now"; backdate the upload so the marker fires.
	_, err := s.Store.DB.Exec(`UPDATE attachment SET uploaded_at = ? WHERE owner_id = ?`,
		timeutil.ToDBUTC(time.Now().UTC().Add(-2*time.Hour)), qid)
	if err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments", qid), nil))
	var rows []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["source_updated"] != true {
		t.Fatalf("source_updated = %v rows=%v", rows, rows)
	}
}

func TestAttachmentIndexIsMetadataOnly(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	a := seedQuest(t, s)
	b := seedQuest(t, s)
	h := s.Handler()

	if w := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", a), "a.txt", "", []byte("aaa")); w.Code != 201 {
		t.Fatalf("upload a = %d %s", w.Code, w.Body.String())
	}
	if w := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", b), "b.txt", "note", []byte("bbb")); w.Code != 201 {
		t.Fatalf("upload b = %d %s", w.Code, w.Body.String())
	}

	idx := httptest.NewRecorder()
	h.ServeHTTP(idx, httptest.NewRequest(http.MethodGet, "/api/attachments", nil))
	if idx.Code != 200 {
		t.Fatalf("index = %d %s", idx.Code, idx.Body.String())
	}
	var grouped map[string]map[string][]map[string]any
	if err := json.Unmarshal(idx.Body.Bytes(), &grouped); err != nil {
		t.Fatal(err)
	}
	if grouped["questline"] == nil || grouped["quest"] == nil {
		t.Fatalf("missing buckets: %v", grouped)
	}
	akey := fmt.Sprintf("%d", a)
	bkey := fmt.Sprintf("%d", b)
	if len(grouped["quest"][akey]) != 1 || grouped["quest"][akey][0]["filename"] != "a.txt" {
		t.Fatalf("quest a = %v", grouped["quest"][akey])
	}
	if grouped["quest"][bkey][0]["comment"] != "note" {
		t.Fatalf("quest b = %v", grouped["quest"][bkey])
	}
	if _, ok := grouped["quest"][akey][0]["available"]; ok {
		t.Fatalf("index must not Stat: available=%v", grouped["quest"][akey][0]["available"])
	}

	quiet := httptest.NewRecorder()
	h.ServeHTTP(quiet, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments?stat=0", a), nil))
	var quietRows []map[string]any
	if err := json.Unmarshal(quiet.Body.Bytes(), &quietRows); err != nil {
		t.Fatal(err)
	}
	if len(quietRows) != 1 {
		t.Fatalf("stat=0 len = %d", len(quietRows))
	}
	if _, ok := quietRows[0]["available"]; ok {
		t.Fatalf("stat=0 must omit available: %v", quietRows[0])
	}

	live := httptest.NewRecorder()
	h.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/api/attachments?stat=1", nil))
	if err := json.Unmarshal(live.Body.Bytes(), &grouped); err != nil {
		t.Fatal(err)
	}
	if grouped["quest"][akey][0]["available"] != true {
		t.Fatalf("stat=1 available = %v", grouped["quest"][akey][0]["available"])
	}
}

// 1x1 PNG; mimetype sniffs it as image/png regardless of filename.
var tinyPNG = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 0x0d, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0xf8, 0xff, 0xff, 0x3f, 0, 5, 0xfe, 2, 0xfe, 0xdc, 0xcc, 0x59, 0xe7,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}

func TestAttachmentImageInline(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	qid := seedQuest(t, s)
	h := s.Handler()

	up := func(name string, body []byte) int64 {
		w := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), name, "", body)
		if w.Code != http.StatusCreated {
			t.Fatalf("upload %s = %d %s", name, w.Code, w.Body.String())
		}
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return int64(m["id"].(float64))
	}
	get := func(aid int64, q string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/attachments/%d/image%s", aid, q), nil))
		return w
	}

	// Owner-less: the quest id is not in the URL.
	png := up("pic.png", tinyPNG)
	w := get(png, "")
	if w.Code != 200 {
		t.Fatalf("image = %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") ||
		w.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("hardening headers missing: %v", w.Header())
	}
	if !bytes.Equal(w.Body.Bytes(), tinyPNG) {
		t.Fatalf("body mismatch")
	}

	// Type comes from the sniff, not the filename: a text file named .png is refused.
	fake := up("fake.png", []byte("just text pretending"))
	if w := get(fake, ""); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text-as-png = %d, want 415", w.Code)
	}
	// SVG is never inlined.
	svg := up("a.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`))
	if w := get(svg, ""); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("svg = %d, want 415", w.Code)
	}
	if w := get(9999, ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing = %d, want 404", w.Code)
	}
	if w := get(png, "?revision=0"); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad revision = %d, want 422", w.Code)
	}
	if w := get(png, "?revision=7"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown revision = %d, want 404", w.Code)
	}
}

func TestAttachmentHidden(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	qid := seedQuest(t, s)
	h := s.Handler()

	w := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "pic.png", "", tinyPNG)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", w.Code, w.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	aid := int64(m["id"].(float64))
	if m["hidden"] != false {
		t.Fatalf("new card hidden = %v, want false", m["hidden"])
	}

	patch := func(body string) *httptest.ResponseRecorder {
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, httptest.NewRequest(http.MethodPatch,
			fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), strings.NewReader(body)))
		return rw
	}
	image := func() int {
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/attachments/%d/image", aid), nil))
		return rw.Code
	}

	if rw := patch(`{"hidden":true}`); rw.Code != 200 || !strings.Contains(rw.Body.String(), `"hidden":true`) {
		t.Fatalf("hide = %d %s", rw.Code, rw.Body.String())
	}
	if c := image(); c != http.StatusNotFound {
		t.Fatalf("hidden image = %d, want 404", c)
	}
	pw := httptest.NewRecorder()
	h.ServeHTTP(pw, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/attachments/%d/image?preview=1", aid), nil))
	if pw.Code != 200 {
		t.Fatalf("hidden preview = %d, want 200", pw.Code)
	}
	// Still listed (the Attachments tab needs it) and still downloadable.
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments?stat=0", qid), nil))
	if !strings.Contains(lw.Body.String(), `"hidden":true`) {
		t.Fatalf("list lost hidden card: %s", lw.Body.String())
	}
	dw := httptest.NewRecorder()
	h.ServeHTTP(dw, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments/%d", qid, aid), nil))
	if dw.Code != 200 {
		t.Fatalf("hidden download = %d", dw.Code)
	}
	// Context payloads (MCP) skip hidden cards.
	if got := s.attachmentsForOwner(t.Context(), ownerQuest, qid, false); len(got) != 0 {
		t.Fatalf("attachmentsForOwner returned %d hidden cards", len(got))
	}

	if rw := patch(`{"hidden":false}`); rw.Code != 200 {
		t.Fatalf("unhide = %d", rw.Code)
	}
	if c := image(); c != 200 {
		t.Fatalf("unhidden image = %d, want 200", c)
	}
}

func TestAttachmentEventsLandInChangeLog(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	qid := seedQuest(t, s)
	h := s.Handler()
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "log.md", "", []byte("# x"))
	if up.Code != 201 {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}
	rows, err := s.Store.ListChangeLog(context.Background(), store.ChangeLogFilter{Limit: 10, EntityTypes: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range rows {
		if r["kind"] == "attachment_added" {
			found = true
			if r["entity_type"] != "attachment" || r["parent_type"] != "quest" || r["parent_id"] != qid || r["title"] != "log.md" {
				t.Fatalf("bad entry: %v", r)
			}
		}
	}
	if !found {
		t.Fatalf("attachment_added missing: %v", rows)
	}
}

func TestLogEventCoalesces(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := context.Background()
	ev := store.ChangeEvent{Kind: "note_updated", EntityType: "note", EntityID: 7, Title: "n", Detail: "изменена", Coalesce: 10 * time.Minute}
	for i := 0; i < 3; i++ {
		if err := s.Store.LogEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := s.Store.ListChangeLog(ctx, store.ChangeLogFilter{Limit: 10, EntityTypes: []string{"all"}})
	if len(rows) != 1 {
		t.Fatalf("want 1 coalesced entry, got %d: %v", len(rows), rows)
	}
}

func TestChangeLogFilters(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := context.Background()
	for i, e := range []store.ChangeEvent{
		{Kind: "note_created", EntityType: "note", EntityID: 1, Title: "Рецепт Борща", Detail: "создана"},
		{Kind: "note_deleted", EntityType: "note", EntityID: 2, Title: "Другое", Detail: "удалена"},
		{Kind: "attachment_added", EntityType: "attachment", EntityID: 3, ParentType: "note", ParentID: 1, Title: "борщ.pdf", Detail: "загружено"},
	} {
		if err := s.Store.LogEvent(ctx, e); err != nil {
			t.Fatal(i, err)
		}
	}
	count := func(f store.ChangeLogFilter) int {
		rows, err := s.Store.ListChangeLog(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}
	all := []string{"all"}
	if n := count(store.ChangeLogFilter{}); n != 0 {
		t.Fatalf("default must be quest-only, got %d", n)
	}
	if n := count(store.ChangeLogFilter{EntityTypes: all}); n != 3 {
		t.Fatalf("all = %d", n)
	}
	if n := count(store.ChangeLogFilter{EntityTypes: []string{"note"}}); n != 2 {
		t.Fatalf("note = %d", n)
	}
	if n := count(store.ChangeLogFilter{EntityTypes: all, Kinds: []string{"note_deleted"}}); n != 1 {
		t.Fatalf("kind = %d", n)
	}
	if n := count(store.ChangeLogFilter{EntityTypes: all, Query: "БОРЩ"}); n != 2 {
		t.Fatalf("cyrillic query = %d", n)
	}
	id := int64(1)
	if n := count(store.ChangeLogFilter{EntityTypes: []string{"note"}, EntityID: &id}); n != 1 {
		t.Fatalf("entity id = %d", n)
	}
	first, _ := s.Store.ListChangeLog(ctx, store.ChangeLogFilter{EntityTypes: all, Limit: 1})
	bid := first[0]["id"].(int64)
	bat, _ := timeutil.ParseFlexible(first[0]["at"].(string))
	if n := count(store.ChangeLogFilter{EntityTypes: all, BeforeID: &bid, BeforeAt: &bat}); n != 2 {
		t.Fatalf("after cursor = %d", n)
	}
}
