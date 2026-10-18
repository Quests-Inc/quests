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

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
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

// The trash is not part of a workspace archive: a deleted quest or note must not travel with the export.
func TestExportSkipsTheTrash(t *testing.T) {
	f := newFixture(t)
	keep, err := f.st.Journal.CreateQuest(f.ctx(), journal.Quest{Title: "Живой"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := f.st.Journal.CreateQuest(f.ctx(), journal.Quest{Title: "Удалённый", Steps: []journal.Step{{Title: "шаг в корзине", ProgressTotal: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	note, err := f.st.Knowledge.CreateNote(f.ctx(), knowledge.Note{Title: "Удалённая заметка"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.Journal.TrashQuest(f.ctx(), gone.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := f.st.Knowledge.TrashNote(f.ctx(), note.ID); err != nil {
		t.Fatal(err)
	}

	w := f.export(t)
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	for _, e := range zr.File {
		if !strings.HasPrefix(e.Name, "tables/") {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		for _, leak := range []string{"Удалённый", "шаг в корзине", "Удалённая заметка"} {
			if strings.Contains(string(body), leak) {
				t.Fatalf("%s carries trashed %q", e.Name, leak)
			}
		}
	}
	_ = keep
}

// A secret of the workspace itself travels with the archive (when secrets are asked for) and lands on the new workspace.
func TestWorkspaceSecretSurvivesExportAndImport(t *testing.T) {
	f := newFixture(t)
	if err := f.st.Execution.SetSecret(f.ctx(), execution.WorkspaceSecretOwner(f.ctx()), "WS_TOKEN", "token-value-1"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	f.h.Export(w, httptest.NewRequest("GET", "/api/workspaces/1/export?files=0&secrets=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("export = %d", w.Code)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "ws.zip")
	_, _ = part.Write(w.Body.Bytes())
	_ = mw.WriteField("name", "С секретами")
	_ = mw.WriteField("secrets", "1")
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/workspaces/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{User: f.user, Via: "session"}))
	rec := httptest.NewRecorder()
	f.h.Import(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}

	var wsID int64
	if err := f.st.DB.QueryRowContext(context.Background(), `SELECT id FROM workspace WHERE name = 'С секретами'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	ctx := wsctx.WithUser(wsctx.With(context.Background(), wsID), f.user.ID)
	got, err := f.st.Execution.ResolveSecrets(ctx, execution.WorkspaceSecretOwner(ctx))
	if err != nil || got["WS_TOKEN"] != "token-value-1" {
		t.Fatalf("the imported workspace's secrets: %v %v", got, err)
	}
}

// Scripts and their versions travel with the workspace.
func TestScriptsSurviveExportAndImport(t *testing.T) {
	f := newFixture(t)
	g := []byte(`{"nodes":[{"id":1,"type":"source.note"},{"id":2,"type":"sink.note_error"},{"id":3,"type":"code","params":{"text":"echo 1"}},{"id":4,"type":"parse","params":{"schema":[{"name":"message","type":"str","required":true}]}}],
		"edges":[{"from":{"node":1,"port":"ref"},"to":{"node":2,"port":"ref"}},{"from":{"node":3,"port":"failed"},"to":{"node":4,"port":"in"}},{"from":{"node":4,"port":"message"},"to":{"node":2,"port":"message"}}]}`)
	sc, err := f.st.Execution.CreateScript(f.ctx(), "Из архива", "", g)
	if err != nil {
		t.Fatal(err)
	}
	g2 := bytes.Replace(g, []byte("echo 1"), []byte("echo 2"), 1)
	if _, err := f.st.Execution.UpdateScript(f.ctx(), sc.ID, execution.ScriptUpdate{Graph: g2, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}

	w := f.export(t)
	rec := httptest.NewRecorder()
	f.h.Import(rec, upload(t, "ws.zip", w.Body.Bytes(), f.user))
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	var wsID int64
	if err := f.st.DB.QueryRowContext(context.Background(), `SELECT id FROM workspace WHERE name = 'Импортированное'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	ctx := wsctx.WithUser(wsctx.With(context.Background(), wsID), f.user.ID)
	list, err := f.st.Execution.ListScripts(ctx)
	if err != nil || len(list) != 1 || list[0].Title != "Из архива" || list[0].Version != 2 {
		t.Fatalf("imported scripts: %+v %v", list, err)
	}
	v1, err := f.st.Execution.ScriptGraph(ctx, list[0].ID, 1)
	if err != nil || !strings.Contains(string(v1), "echo 1") {
		t.Fatalf("version 1 of the imported script: %s %v", v1, err)
	}
}

// A script that holds a custom node and reads a note by id follows them to the new workspace (quest=458).
func TestScriptGraphIDsFollowTheImport(t *testing.T) {
	f := newFixture(t)
	ctx := f.ctx()
	// A throw-away note and node, so the ids of the real ones are not 1.
	if _, err := f.st.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Лишняя"}); err != nil {
		t.Fatal(err)
	}
	note, err := f.st.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Проверяемая"})
	if err != nil {
		t.Fatal(err)
	}
	inner := graph.Graph{
		Nodes: []graph.Node{{ID: 1, Type: "source.note", Params: []byte(`{"id":` + itoa(note.ID) + `}`)}, {ID: 2, Type: "sink.done"}},
		Edges: []graph.Edge{{From: graph.End{Node: 1, Port: "ref"}, To: graph.End{Node: 2, Port: "trigger"}}},
	}
	if _, err := f.st.Execution.CreateCustomNode(ctx, graph.Custom{Title: "Лишняя нода", Graph: inner}); err != nil {
		t.Fatal(err)
	}
	node, err := f.st.Execution.CreateCustomNode(ctx, graph.Custom{
		Title:  "Проверка заметки",
		Fields: []graph.CustomField{{Name: "note", Type: "int", Default: note.ID, Node: 1, Param: "id"}},
		Graph:  inner,
	})
	if err != nil {
		t.Fatal(err)
	}
	g := []byte(`{"nodes":[{"id":1,"type":"custom.` + itoa(node.ID) + `"},{"id":2,"type":"source.note","params":{"id":` + itoa(note.ID) + `}},{"id":3,"type":"sink.done"}],
		"edges":[{"from":{"node":2,"port":"ref"},"to":{"node":3,"port":"trigger"}}]}`)
	if _, err := f.st.Execution.CreateScript(ctx, "С нодой", "", g); err != nil {
		t.Fatal(err)
	}

	w := f.export(t)
	rec := httptest.NewRecorder()
	f.h.Import(rec, upload(t, "ws.zip", w.Body.Bytes(), f.user))
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	var wsID int64
	if err := f.st.DB.QueryRowContext(context.Background(), `SELECT id FROM workspace WHERE name = 'Импортированное'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	ictx := wsctx.WithUser(wsctx.With(context.Background(), wsID), f.user.ID)
	notes, _ := f.st.Knowledge.ListNotes(ictx, knowledge.NoteFilter{})
	var newNote int64
	for _, n := range notes {
		if n.Title == "Проверяемая" {
			newNote = n.ID
		}
	}
	nodes, err := f.st.Execution.ListCustomNodes(ictx)
	if err != nil || len(nodes) != 2 || newNote == 0 || newNote == note.ID {
		t.Fatalf("imported nodes %+v note %d (was %d): %v", nodes, newNote, note.ID, err)
	}
	var imported execution.CustomNode
	for _, n := range nodes {
		if n.Title == "Проверка заметки" {
			imported = n
		}
	}
	if imported.Problem != "" || string(imported.Graph.Nodes[0].Params) != `{"id":`+itoa(newNote)+`}` || imported.Fields[0].Default != float64(newNote) {
		t.Fatalf("the node's inside points at the new note: %+v", imported)
	}
	scripts, _ := f.st.Execution.ListScripts(ictx)
	raw, err := f.st.Execution.ScriptGraph(ictx, scripts[0].ID, scripts[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"custom.`+itoa(imported.ID)+`"`) || !strings.Contains(string(raw), `"id":`+itoa(newNote)) {
		t.Fatalf("the script's graph follows the import: %s", raw)
	}
	if _, err := f.st.Execution.CreateScript(ictx, "ещё раз", "", raw); err != nil {
		t.Fatalf("the imported graph is a valid script: %v", err)
	}
}
