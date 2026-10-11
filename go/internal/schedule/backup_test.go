package schedule

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/health"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/store"
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

// openBackupDB is a real database; pg_dump itself is replaced by a stub that writes a
// small file (the dump is exercised by hand/CI against a real server, not here).
func openBackupDB(t *testing.T) *store.Store {
	t.Helper()
	orig := dumpDatabase
	dumpDatabase = func(_ context.Context, url, outPath string) error {
		if url == "" {
			return errors.New("no database url")
		}
		return os.WriteFile(outPath, []byte("-- dump of "+url), 0o600)
	}
	t.Cleanup(func() { dumpDatabase = orig })
	pool, url := pgtest.NewWithURL(t)
	return store.New(pool, url, nil)
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
		VALUES ('quest', 1, 'a.txt', 'attachments/quest-1/a.txt', 12, now(), 'clean', 1)`); err != nil {
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
	got := attachmentsArchiveName("quests-20260930-120000-123456789.dump")
	want := "quests-20260930-120000-123456789-attachments.tar.zst"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreMigrationBackupWritesAndPrunes(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pg_dump"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	orig := dumpDatabase
	dumpDatabase = func(_ context.Context, _, out string) error { return os.WriteFile(out, []byte("dump"), 0o600) }
	t.Cleanup(func() { dumpDatabase = orig })

	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, backupSubdir)
	_ = os.MkdirAll(dir, 0o755)
	for _, old := range []string{"20250101-000000", "20250102-000000", "20250103-000000"} {
		_ = os.WriteFile(filepath.Join(dir, "pre-migrate-a-to-b-"+old+".dump"), []byte("x"), 0o600)
	}
	path, err := PreMigrationBackup(context.Background(), "postgresql://x", dataDir, "0010", "0012", 2, t.Logf)
	if err != nil || !strings.Contains(filepath.Base(path), "pre-migrate-0010-to-0012-") {
		t.Fatalf("path=%q err=%v", path, err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "pre-migrate-*.dump"))
	if len(left) != 2 {
		t.Fatalf("want the newest 2 kept, got %v", left)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the new dump was pruned: %v", err)
	}
}

func TestPreMigrationBackupSkipsWithoutPgDump(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	warned := false
	path, err := PreMigrationBackup(context.Background(), "postgresql://x", t.TempDir(), "a", "b", 5, func(string, ...any) { warned = true })
	if err != nil || path != "" || !warned {
		t.Fatalf("path=%q err=%v warned=%v", path, err, warned)
	}
}
