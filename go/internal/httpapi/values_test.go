package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestNamedValuesAPIAndEmbed(t *testing.T) {
	h, s := secretsServer(t)
	ctx := wsctx.With(t.Context(), 1)
	parent, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Герой"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Сцена", ParentID: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/notes/" + strconv.FormatInt(parent.ID, 10)
	if code, out := secretReq(h, http.MethodPut, base+"/values/str", map[string]any{"type": "int", "value": 3}); code != http.StatusOK || out["value"] != float64(3) {
		t.Fatalf("put: %d %v", code, out)
	}
	if code, _ := secretReq(h, http.MethodPut, base+"/values/Path", map[string]any{"type": "str", "value": "x"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("reserved name: %d", code)
	}
	childURL := "/api/notes/" + strconv.FormatInt(child.ID, 10) + "/values"
	code, list := secretReq(h, http.MethodGet, childURL, nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d %v", code, list)
	}
	inh, _ := list["inherited"].([]any)
	if len(inh) != 1 || inh[0].(map[string]any)["name"] != "str" {
		t.Fatalf("inherited = %v", list["inherited"])
	}

	if code, _ := secretReq(h, http.MethodPut, "/api/workspace/values/dc", map[string]any{"type": "int", "value": 15}); code != http.StatusOK {
		t.Fatalf("workspace: %d", code)
	}

	const graph = `{"nodes":[
		{"id":1,"type":"args","params":{"params":[{"name":"expr","type":"str"}]}},
		{"id":2,"type":"expr","params":{}},
		{"id":3,"type":"sink.result","params":{}}],
		"edges":[
		{"from":{"node":1,"port":"expr"},"to":{"node":2,"port":"expr"}},
		{"from":{"node":2,"port":"text"},"to":{"node":3,"port":"value"}}]}`
	code, sc := secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "сумма", "graph": jsonRaw(t, graph)})
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	id := jsonNum(sc["id"])
	code, ev := secretReq(h, http.MethodPost, "/api/scripts/"+id+"/eval", map[string]any{
		"args": map[string]string{"_": "1+str+dc"},
		"host": map[string]any{"kind": "note", "id": child.ID},
	})
	if code != http.StatusOK || ev["value"] != "19" {
		t.Fatalf("eval: %d %v", code, ev)
	}
	code, ev = secretReq(h, http.MethodPost, "/api/scripts/"+id+"/eval", map[string]any{
		"args": map[string]string{"_": "note=" + strconv.FormatInt(parent.ID, 10) + ".str"},
		"host": map[string]any{"kind": "note", "id": child.ID},
	})
	if code != http.StatusOK || ev["value"] != "3" {
		t.Fatalf("foreign: %d %v", code, ev)
	}
}

func TestNamedFormulaFeedsScript(t *testing.T) {
	h, s := secretsServer(t)
	ctx := wsctx.With(t.Context(), 1)
	note, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Волк"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/notes/" + strconv.FormatInt(note.ID, 10)
	if code, _ := secretReq(h, http.MethodPut, base+"/values/str_score", map[string]any{"type": "int", "value": 12}); code != http.StatusOK {
		t.Fatalf("score: %d", code)
	}
	code, out := secretReq(h, http.MethodPut, base+"/values/str", map[string]any{"type": "int", "formula": "(str_score - 10) / 2"})
	if code != http.StatusOK || out["value"] != float64(1) || out["formula"] != "(str_score - 10) / 2" {
		t.Fatalf("formula: %d %v", code, out)
	}
	if code, out = secretReq(h, http.MethodPut, base+"/values/bad", map[string]any{"type": "int", "formula": "nope + 1"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad formula: %d %v", code, out)
	}
	const graph = `{"nodes":[
		{"id":1,"type":"args","params":{"params":[{"name":"expr","type":"str"}]}},
		{"id":2,"type":"expr","params":{}},
		{"id":3,"type":"sink.result","params":{}}],
		"edges":[
		{"from":{"node":1,"port":"expr"},"to":{"node":2,"port":"expr"}},
		{"from":{"node":2,"port":"text"},"to":{"node":3,"port":"value"}}]}`
	code, sc := secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "мод", "graph": jsonRaw(t, graph)})
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	code, ev := secretReq(h, http.MethodPost, "/api/scripts/"+jsonNum(sc["id"])+"/eval", map[string]any{
		"args": map[string]string{"_": "str"},
		"host": map[string]any{"kind": "note", "id": note.ID},
	})
	if code != http.StatusOK || ev["value"] != "1" {
		t.Fatalf("eval: %d %v", code, ev)
	}
	if code, _ = secretReq(h, http.MethodPut, base+"/values/pb", map[string]any{"type": "int", "value": 2}); code != http.StatusOK {
		t.Fatalf("pb: %d", code)
	}
	code, ev = secretReq(h, http.MethodPost, "/api/scripts/"+jsonNum(sc["id"])+"/eval", map[string]any{
		"args": map[string]string{"_": "str+pb"},
		"host": map[string]any{"kind": "note", "id": note.ID},
	})
	if code != http.StatusOK || ev["value"] != "3" {
		t.Fatalf("eval: %d %v", code, ev)
	}
	if code, _ = secretReq(h, http.MethodPut, base+"/values/str_score", map[string]any{"type": "int", "value": 8}); code != http.StatusOK {
		t.Fatalf("rescore: %d", code)
	}
	code, ev = secretReq(h, http.MethodPost, "/api/scripts/"+jsonNum(sc["id"])+"/eval", map[string]any{
		"args": map[string]string{"_": "str"},
		"host": map[string]any{"kind": "note", "id": note.ID},
	})
	if code != http.StatusOK || ev["value"] != "-1" {
		t.Fatalf("recomputed: %d %v", code, ev)
	}
}

func TestRollWritesLiteralAndFormulasFollow(t *testing.T) {
	h, s := secretsServer(t)
	ctx := wsctx.With(t.Context(), 1)
	note, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Волк"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/notes/" + strconv.FormatInt(note.ID, 10)
	put := func(name string, body map[string]any) (int, map[string]any) {
		return secretReq(h, http.MethodPut, base+"/values/"+name, body)
	}
	put("pb", map[string]any{"type": "int", "value": 2})
	put("hp", map[string]any{"type": "int", "value": 1})
	if code, out := put("half", map[string]any{"type": "int", "formula": "hp / 2"}); code != http.StatusOK {
		t.Fatalf("half: %d %v", code, out)
	}
	const graph = `{"nodes":[
		{"id":1,"type":"args","params":{"params":[{"name":"expr","type":"str"}]}},
		{"id":2,"type":"expr","params":{}},
		{"id":3,"type":"sink.result","params":{}}],
		"edges":[
		{"from":{"node":1,"port":"expr"},"to":{"node":2,"port":"expr"}},
		{"from":{"node":2,"port":"text"},"to":{"node":3,"port":"value"}}]}`
	code, sc := secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "бросок", "graph": jsonRaw(t, graph)})
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	script := int64(sc["id"].(float64))
	roll := func(name string, body any) (int, map[string]any) {
		return secretReq(h, http.MethodPost, base+"/values/"+name+"/roll", body)
	}

	if code, out := roll("hp", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("no source: %d %v", code, out)
	}
	if code, out := roll("hpp", map[string]any{"script": script, "arg": "pb*3"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("a typo must not create a value: %d %v", code, out)
	}
	if code, out := roll("half", map[string]any{"script": script, "arg": "pb*3"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("a formula is not replaced: %d %v", code, out)
	}
	code, out := roll("hp", map[string]any{"script": script, "arg": "pb*3"})
	roll1, _ := out["roll"].(map[string]any)
	if code != http.StatusOK || out["value"] != float64(6) || roll1["arg"] != "pb*3" {
		t.Fatalf("roll: %d %v", code, out)
	}
	_, list := secretReq(h, http.MethodGet, base+"/values", nil)
	for _, row := range list["effective"].([]any) {
		if r := row.(map[string]any); r["name"] == "half" && r["value"] != float64(3) {
			t.Fatalf("a dependent formula must follow: %v", r)
		}
	}
	put("pb", map[string]any{"type": "int", "value": 4})
	if code, out = roll("hp", nil); code != http.StatusOK || out["value"] != float64(12) {
		t.Fatalf("again from the stored source: %d %v", code, out)
	}
	if _, out = put("hp", map[string]any{"type": "int", "value": 9}); out["roll"] != nil {
		t.Fatalf("a typed-in value forgets the roll: %v", out)
	}
	if code, out = roll("hp", map[string]any{"script": script, "arg": "pb*0.3"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("a float result for an int: %d %v", code, out)
	}
}

func jsonRaw(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNetworkNodeNeedsThePolicyAndTheGrant(t *testing.T) {
	const g = `{"nodes":[
		{"id":1,"type":"args","params":{"params":[{"name":"q","type":"str"}]}},
		{"id":2,"type":"http.get","params":{"host":"api.example.org"}},
		{"id":3,"type":"json.get","params":{"path":"a","type":"str"}},
		{"id":4,"type":"sink.result","params":{}}],
		"edges":[
		{"from":{"node":1,"port":"q"},"to":{"node":2,"port":"path"}},
		{"from":{"node":2,"port":"body"},"to":{"node":3,"port":"in"}},
		{"from":{"node":3,"port":"value"},"to":{"node":4,"port":"value"}}]}`
	save := func(h http.Handler) (int, map[string]any) {
		return secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "сеть", "graph": jsonRaw(t, g)})
	}
	// No policy: the node cannot be saved, and a check says so without saving.
	h, _ := secretsServer(t)
	if code, out := save(h); code != http.StatusUnprocessableEntity || out["detail"] == nil {
		t.Fatalf("no policy: %d %v", code, out)
	}
	code, v := secretReq(h, http.MethodPost, "/api/scripts/validate", map[string]any{"graph": jsonRaw(t, g)})
	if code != http.StatusOK || v["ok"] != false {
		t.Fatalf("validate without policy: %d %v", code, v)
	}

	// The policy is open but the workspace has no grant: still refused, and the grant is the administrator's alone. The execution
	// handler is built on the first request, so this server gets the policy before it.
	pol, err := egress.PolicyFromEnv(func(k string) string {
		if k == "QUESTS_EGRESS_MODE" {
			return "public"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	h2, s2 := secretsServerWith(t, func(s *Server) { s.Egress = egress.New(pol, nil) })
	ctx := wsctx.With(t.Context(), 1)
	if code, _ := save(h2); code != http.StatusUnprocessableEntity {
		t.Fatalf("no grant: %d", code)
	}
	// The API role reads the grant but cannot write it: the administrator does, with the CLI, as the database owner.
	if err := s2.Store.Execution.SetEgressGrant(ctx, 1, true); err == nil {
		t.Fatal("the API role must not be able to grant itself outgoing HTTP")
	}
	owner, err := sql.Open("pgx", s2.Store.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.Exec(`INSERT INTO egress_grant (workspace_id, enabled, updated_at) VALUES (1, 1, now())`); err != nil {
		t.Fatal(err)
	}
	if !s2.Store.Execution.EgressEnabled(ctx) {
		t.Fatal("the grant is read back")
	}
	if code, out := save(h2); code != http.StatusCreated {
		t.Fatalf("granted: %d %v", code, out)
	}
}
