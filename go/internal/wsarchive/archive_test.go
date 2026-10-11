package wsarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

type fixture struct {
	h    *Handler
	st   *store.Store
	user identity.User
	ws   identity.Workspace
	role string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	st := store.New(pool, url, nil)
	user, err := st.Identity.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := st.Identity.CreateWorkspace(context.Background(), user.ID, "Исходное")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{st: st, user: user, ws: ws, role: identity.RoleOwner}
	f.h = &Handler{
		Store: st,
		MemberOf: func(w http.ResponseWriter, r *http.Request) (int64, string, identity.User, bool) {
			return f.ws.ID, f.role, f.user, true
		},
		OverQuota: func(context.Context, int64) (bool, int64, error) { return false, 0, nil },
	}
	return f
}

func (f *fixture) ctx() context.Context {
	return wsctx.WithUser(wsctx.With(context.Background(), f.ws.ID), f.user.ID)
}

func (f *fixture) export(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	f.h.Export(w, httptest.NewRequest("GET", "/api/workspaces/1/export?files=0", nil))
	return w
}

func upload(t *testing.T, name string, body []byte, user identity.User) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", name)
	_, _ = part.Write(body)
	_ = mw.WriteField("name", "Импортированное")
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/workspaces/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{User: user, Via: "session"}))
}

func TestExportOnlyForOwners(t *testing.T) {
	f := newFixture(t)
	f.role = identity.RoleMember
	if w := f.export(t); w.Code != http.StatusForbidden {
		t.Fatalf("member export = %d, want 403", w.Code)
	}
}

func TestExportThenImportRoundTrip(t *testing.T) {
	f := newFixture(t)
	q, err := f.st.Journal.CreateQuest(f.ctx(), journal.Quest{Title: "Купить хлеб", Steps: []journal.Step{{Title: "Дойти до магазина", ProgressTotal: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.Knowledge.CreateNote(f.ctx(), knowledge.Note{Title: "Рецепт", Description: "см. quest=" + itoa(q.ID)}); err != nil {
		t.Fatal(err)
	}

	w := f.export(t)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("export = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	names := map[string]bool{}
	for _, e := range zr.File {
		names[e.Name] = true
	}
	for _, want := range []string{"manifest.json", "tables/quest.jsonl", "tables/note.jsonl"} {
		if !names[want] {
			t.Fatalf("archive lacks %s (has %v)", want, names)
		}
	}

	rec := httptest.NewRecorder()
	f.h.Import(rec, upload(t, "ws.zip", w.Body.Bytes(), f.user))
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Импортированное") {
		t.Fatalf("import response: %s", rec.Body.String())
	}
	// A new workspace now holds the same quest and a note whose link follows the renumbered quest.
	var wsID int64
	if err := f.st.DB.QueryRowContext(context.Background(), `SELECT id FROM workspace WHERE name = 'Импортированное'`).Scan(&wsID); err != nil {
		t.Fatalf("imported workspace: %v", err)
	}
	ctx := wsctx.WithUser(wsctx.With(context.Background(), wsID), f.user.ID)
	quests, err := f.st.Journal.ListQuests(ctx, journal.ListFilter{})
	if err != nil || len(quests) != 1 || quests[0].Title != "Купить хлеб" || len(quests[0].Steps) != 1 {
		t.Fatalf("imported quests = %+v, err %v", quests, err)
	}
	notes, err := f.st.Knowledge.ListNotes(ctx, knowledge.NoteFilter{})
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0].Description, "quest="+itoa(quests[0].ID)) {
		t.Fatalf("imported notes = %+v, err %v", notes, err)
	}
}

func TestImportRefusesBadArchives(t *testing.T) {
	f := newFixture(t)
	post := func(body []byte, user identity.User) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.h.Import(w, upload(t, "x.zip", body, user))
		return w
	}
	if w := post([]byte("не zip"), f.user); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("garbage = %d, want 422", w.Code)
	}
	var empty bytes.Buffer
	zw := zip.NewWriter(&empty)
	_ = zw.Close()
	if w := post(empty.Bytes(), f.user); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "manifest.json") {
		t.Errorf("no manifest = %d %s", w.Code, w.Body.String())
	}
	for _, c := range []struct{ name, manifest, want string }{
		{"format", `{"format":"other","version":1}`, "неизвестный формат"},
		{"version", `{"format":"` + store.ExportFormat + `","version":999}`, "новее"},
		{"broken", `{`, "повреждён"},
	} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		mf, _ := zw.Create("manifest.json")
		_, _ = io.WriteString(mf, c.manifest)
		_ = zw.Close()
		if w := post(buf.Bytes(), f.user); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
	}
	// No account behind the request: nothing to own the new workspace.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/workspaces/import", strings.NewReader(""))
	f.h.Import(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("anonymous import = %d, want 400", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
