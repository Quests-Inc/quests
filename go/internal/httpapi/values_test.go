package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
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

func jsonRaw(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
