package schedule

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/valirum/quests/go/internal/health"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/webdav"
)

// newFakeDAV is a minimal in-memory WebDAV stand-in — Put/Get/Delete for backup.
func newFakeDAV(t *testing.T) (*httptest.Server, *sync.Map) {
	t.Helper()
	files := &sync.Map{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Path, "/")
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			files.Store(path, body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			v, ok := files.Load(path)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(v.([]byte))
		case http.MethodDelete:
			_, ok := files.LoadAndDelete(path)
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, files
}

const backupTestSchema = `
CREATE TABLE quest (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL
);
CREATE TABLE attachment (
	id INTEGER PRIMARY KEY,
	owner_type TEXT NOT NULL,
	owner_id INTEGER NOT NULL,
	filename TEXT NOT NULL,
	webdav_path TEXT NOT NULL,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	content_type_declared TEXT,
	content_type_detected TEXT,
	comment TEXT,
	uploaded_at DATETIME,
	scan_status TEXT,
	scanned_at DATETIME,
	current_revision INTEGER NOT NULL DEFAULT 1,
	hidden INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE attachment_revision (
	id INTEGER PRIMARY KEY,
	attachment_id INTEGER NOT NULL,
	revision INTEGER NOT NULL,
	filename TEXT NOT NULL,
	webdav_path TEXT NOT NULL,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	content_type_declared TEXT,
	content_type_detected TEXT,
	comment TEXT,
	uploaded_at DATETIME,
	scan_status TEXT,
	scanned_at DATETIME
);
CREATE TABLE backuplog (
	id INTEGER PRIMARY KEY,
	filename TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	created_at DATETIME NOT NULL,
	webdav_uploaded INTEGER NOT NULL DEFAULT 0,
	remote_uploaded INTEGER NOT NULL DEFAULT 0,
	attachments_filename TEXT,
	attachments_size_bytes INTEGER,
	error TEXT
);
`

type memRemote struct {
	mu    sync.Mutex
	files map[string][]byte
	fail  bool
}

func newMemRemote() *memRemote {
	return &memRemote{files: map[string][]byte{}}
}

func (m *memRemote) Configured() bool { return true }

func (m *memRemote) Upload(_ context.Context, localPath, remoteName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return context.DeadlineExceeded
	}
	b, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	m.files[remoteName] = b
	return nil
}

func (m *memRemote) Remove(_ context.Context, remoteName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, remoteName)
	return nil
}

func (m *memRemote) has(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[name]
	return ok
}

func (m *memRemote) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.files)
}

func openBackupDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:backup_"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(backupTestSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO quest (title) VALUES ('seed')`); err != nil {
		t.Fatal(err)
	}
	return &store.Store{DB: db}
}

func TestRunBackupOnceSnapshotsAndRecords(t *testing.T) {
	st := openBackupDB(t)
	dataDir := t.TempDir()
	reg := health.New()

	if err := RunBackupOnce(context.Background(), st, webdav.New("", "", ""), dataDir, 14, noopRemote{}, reg); err != nil {
		t.Fatalf("RunBackupOnce: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, backupSubdir))
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 snapshot file, got %d", len(entries))
	}

	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatalf("ListBackupLogs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 backuplog row, got %d", len(rows))
	}
	if rows[0].WebdavUploaded {
		t.Fatal("unconfigured webdav should not mark uploaded")
	}
	if rows[0].RemoteUploaded {
		t.Fatal("unconfigured remote should not mark uploaded")
	}
	if rows[0].SizeBytes <= 0 {
		t.Fatal("snapshot should have nonzero size")
	}
	if rows[0].AttachmentsFilename != "" {
		t.Fatal("no webdav → no attachments archive")
	}
}

func TestRunBackupOncePrunesToKeep(t *testing.T) {
	st := openBackupDB(t)
	dataDir := t.TempDir()
	reg := health.New()
	wd := webdav.New("", "", "")

	for i := 0; i < 3; i++ {
		if err := RunBackupOnce(context.Background(), st, wd, dataDir, 2, noopRemote{}, reg); err != nil {
			t.Fatalf("RunBackupOnce #%d: %v", i, err)
		}
	}

	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatalf("ListBackupLogs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows after pruning to keep=2, got %d", len(rows))
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, backupSubdir))
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 files on disk after pruning, got %d", len(entries))
	}
}

func TestRunBackupOnceUploadsAndPrunesWebdav(t *testing.T) {
	st := openBackupDB(t)
	dataDir := t.TempDir()
	reg := health.New()
	dav, _ := newFakeDAV(t)
	wd := webdav.New(dav.URL, "", "")

	for i := 0; i < 3; i++ {
		if err := RunBackupOnce(context.Background(), st, wd, dataDir, 2, noopRemote{}, reg); err != nil {
			t.Fatalf("RunBackupOnce #%d: %v", i, err)
		}
	}

	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatalf("ListBackupLogs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows after pruning to keep=2, got %d", len(rows))
	}
	for _, r := range rows {
		if !r.WebdavUploaded {
			t.Fatalf("row %d (%s) should be marked webdav_uploaded", r.ID, r.Filename)
		}
		if r.AttachmentsFilename == "" {
			t.Fatal("webdav configured → expect attachments archive name")
		}
	}
}

func TestRunBackupOnceArchivesAttachmentsAndRemote(t *testing.T) {
	st := openBackupDB(t)
	dataDir := t.TempDir()
	reg := health.New()
	dav, files := newFakeDAV(t)
	wd := webdav.New(dav.URL, "", "")

	files.Store("attachments/quest-1/a.txt", []byte("hello-attach"))
	if _, err := st.DB.Exec(`
		INSERT INTO attachment (owner_type, owner_id, filename, webdav_path, size_bytes, uploaded_at, scan_status, current_revision)
		VALUES ('quest', 1, 'a.txt', 'attachments/quest-1/a.txt', 12, datetime('now'), 'clean', 1)`); err != nil {
		t.Fatal(err)
	}

	remote := newMemRemote()
	if err := RunBackupOnce(context.Background(), st, wd, dataDir, 14, remote, reg); err != nil {
		t.Fatalf("RunBackupOnce: %v", err)
	}

	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.AttachmentsFilename == "" || r.AttachmentsSizeBytes <= 0 {
		t.Fatalf("attachments archive missing: %+v", r)
	}
	if !r.RemoteUploaded {
		t.Fatal("remote should be marked uploaded")
	}
	if !remote.has(r.Filename) || !remote.has(r.AttachmentsFilename) {
		t.Fatalf("remote missing pair: db=%v att=%v", remote.has(r.Filename), remote.has(r.AttachmentsFilename))
	}
}

func TestRunBackupOnceFlushesPendingRemote(t *testing.T) {
	st := openBackupDB(t)
	dataDir := t.TempDir()
	reg := health.New()
	remote := newMemRemote()
	remote.fail = true

	if err := RunBackupOnce(context.Background(), st, webdav.New("", "", ""), dataDir, 14, remote, reg); err != nil {
		t.Fatalf("RunBackupOnce: %v", err)
	}
	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].RemoteUploaded {
		t.Fatal("expected remote_uploaded=0 while failing")
	}
	if remote.len() != 0 {
		t.Fatal("failing remote should not store files")
	}

	remote.fail = false
	if err := RunBackupOnce(context.Background(), st, webdav.New("", "", ""), dataDir, 14, remote, reg); err != nil {
		t.Fatalf("RunBackupOnce flush: %v", err)
	}
	rows, err = st.ListBackupLogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, r := range rows {
		if !r.RemoteUploaded {
			pending++
		}
	}
	if pending != 0 {
		t.Fatalf("after flush want 0 pending, got %d", pending)
	}
	if !remote.has(rows[0].Filename) {
		// newest is first; both should be on remote
		t.Fatal("newest db missing on remote")
	}
}

func TestAttachmentsArchiveName(t *testing.T) {
	got := attachmentsArchiveName("quests-20260930-120000-123456789.db")
	want := "quests-20260930-120000-123456789-attachments.tar.zst"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
