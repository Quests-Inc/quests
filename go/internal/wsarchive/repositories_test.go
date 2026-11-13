package wsarchive

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/repository"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRepositoryArchiveRoundTrip(t *testing.T) {
	f := newFixture(t)
	f.h.DataDir = t.TempDir()
	ctx := f.ctx()
	q, err := f.st.Journal.CreateQuest(ctx, journal.Quest{Title: "Source quest", Status: "active", Significance: "common"})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = f.st.DB.QueryRowContext(ctx, `INSERT INTO repository(title,kind,branch) VALUES('Mini','local','main') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = f.st.DB.ExecContext(ctx, `INSERT INTO repository_link(repository_id,owner_type,owner_id,path) VALUES(?,'quest',?,'hello.txt')`, id, q.ID); err != nil {
		t.Fatal(err)
	}
	dir := repository.Directory(f.h.DataDir, f.ws.ID, id)
	if err = repository.Init(ctx, dir); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(t.TempDir(), "clone")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	git("clone", dir, local)
	if err = os.WriteFile(filepath.Join(local, "hello.txt"), []byte("source archive\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("-C", local, "add", "hello.txt")
	git("-C", local, "-c", "user.name=Archive", "-c", "user.email=a@example.test", "commit", "-m", "Archive source")
	git("-C", local, "push", "origin", "HEAD:main")
	exported := f.export(t)
	if exported.Code != 200 {
		t.Fatal(exported.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range zr.File {
		if file.Name == "repositories/"+strconv.FormatInt(id, 10)+".bundle" {
			found = true
		}
	}
	if !found {
		t.Fatal("bundle missing")
	}
	w := httptest.NewRecorder()
	f.h.Import(w, upload(t, "workspace.zip", exported.Body.Bytes(), f.user))
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		WorkspaceID int64 `json:"workspace_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	imported := wsctx.With(ctx, result.WorkspaceID)
	var repoID, questID int64
	var path string
	if err = f.st.DB.QueryRowContext(imported, `SELECT r.id,l.owner_id,l.path FROM repository r JOIN repository_link l ON l.repository_id=r.id`).Scan(&repoID, &questID, &path); err != nil {
		t.Fatal(err)
	}
	if repoID == id || questID == q.ID || path != "hello.txt" {
		t.Fatal("ids/links not remapped")
	}
	raw, err := repository.Git(ctx, repository.Directory(f.h.DataDir, result.WorkspaceID, repoID), "show", "refs/heads/main:hello.txt")
	if err != nil || string(raw) != "source archive\n" {
		t.Fatalf("restored %s %v", raw, err)
	}
}
