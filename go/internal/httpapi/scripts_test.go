package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
)

const scriptBody = `{"nodes":[
	{"id":1,"type":"source.note"},
	{"id":2,"type":"code","params":{"text":"echo 1"}},
	{"id":3,"type":"parse","params":{"schema":[{"name":"outdated","type":"bool","required":true}]}},
	{"id":4,"type":"sink.note_verdict"}],
 "edges":[
	{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"in"}},
	{"from":{"node":1,"port":"ref"},"to":{"node":4,"port":"ref"}},
	{"from":{"node":3,"port":"outdated"},"to":{"node":4,"port":"outdated"}}]}`

func TestScriptsOverHTTP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}

	code, out := call(http.MethodPost, "/api/scripts", `{"title":"Заметка свежая","description":"d","graph":`+scriptBody+`}`)
	if code != http.StatusCreated || out["version"] != float64(1) || out["needs_runner"] != true {
		t.Fatalf("create: %d %v", code, out)
	}
	id := "/api/scripts/" + jsonNum(out["id"])

	// A graph that does not pass is a 422 that names the problems.
	code, out = call(http.MethodPost, "/api/scripts", `{"title":"x","graph":{"nodes":[{"id":1,"type":"nope"}],"edges":[]}}`)
	if code != http.StatusUnprocessableEntity || len(out["problems"].([]any)) == 0 {
		t.Fatalf("invalid graph: %d %v", code, out)
	}
	if code, _ = call(http.MethodPost, "/api/scripts", `{"title":"","graph":`+scriptBody+`}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("no title: %d", code)
	}

	code, list := secretReqList(t, h, "/api/scripts")
	if code != http.StatusOK || len(list) != 1 || list[0]["graph"] != nil {
		t.Fatalf("list: %d %v", code, list)
	}
	if code, out = call(http.MethodGet, id, ""); code != http.StatusOK || out["graph"] == nil {
		t.Fatalf("get: %d %v", code, out)
	}

	// A new graph is version 2; the editor that read version 1 is then told it is stale.
	newGraph := strings.Replace(scriptBody, "echo 1", "echo 2", 1)
	code, out = call(http.MethodPut, id, `{"version":1,"graph":`+newGraph+`}`)
	if code != http.StatusOK || out["version"] != float64(2) {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, _ = call(http.MethodPut, id, `{"version":1,"title":"stale"}`); code != http.StatusConflict {
		t.Fatalf("stale: %d, want 409", code)
	}
	code, out = call(http.MethodGet, id+"/versions/1", "")
	if code != http.StatusOK || !strings.Contains(string(mustJSON(out["graph"])), "echo 1") {
		t.Fatalf("version 1: %d %v", code, out)
	}
	if code, _ = call(http.MethodGet, id+"/versions/9", ""); code != http.StatusNotFound {
		t.Fatalf("missing version: %d", code)
	}

	if code, _ = call(http.MethodDelete, id, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = call(http.MethodGet, id, ""); code != http.StatusNotFound {
		t.Fatalf("after delete: %d", code)
	}
}

func TestScriptValidateAndNodeCatalog(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	code, out := secretReq(h, http.MethodPost, "/api/scripts/validate", json.RawMessage(`{"graph":`+scriptBody+`}`))
	if code != http.StatusOK || out["ok"] != true || out["needs_runner"] != true || out["subjects"].([]any)[0] != "note" {
		t.Fatalf("validate a good graph: %d %v", code, out)
	}
	code, out = secretReq(h, http.MethodPost, "/api/scripts/validate", json.RawMessage(`{"graph":{"nodes":[],"edges":[]}}`))
	if code != http.StatusOK || out["ok"] != false || len(out["problems"].([]any)) == 0 {
		t.Fatalf("validate an empty graph: %d %v", code, out)
	}
	if code, out = secretReq(h, http.MethodPost, "/api/scripts/validate", json.RawMessage(`{"graph":"nope"}`)); out["ok"] != false {
		t.Fatalf("validate a malformed graph: %d %v", code, out)
	}

	// The samples are valid scripts: creating one as it is must work.
	code, samples := secretReqList(t, h, "/api/script-samples")
	if code != http.StatusOK || len(samples) == 0 {
		t.Fatalf("samples: %d %v", code, samples)
	}
	for _, sm := range samples {
		body, _ := json.Marshal(map[string]any{"title": sm["title"], "graph": sm["graph"]})
		if code, out := secretReq(h, http.MethodPost, "/api/scripts", json.RawMessage(body)); code != http.StatusCreated {
			t.Errorf("sample %v does not create: %d %v", sm["id"], code, out)
		}
	}

	code, nodes := secretReqList(t, h, "/api/script-nodes")
	if code != http.StatusOK || len(nodes) != 25 || nodes[0]["kind"] != "source" {
		t.Fatalf("catalog: %d (%d types) %v", code, len(nodes), nodes)
	}

	// Ports follow the parameters: a Parse node has an output per schema field.
	code, out = secretReq(h, http.MethodPost, "/api/script-nodes/ports",
		json.RawMessage(`{"type":"parse","params":{"schema":[{"name":"progress","type":"int"},{"name":"description","type":"str"}]}}`))
	outs, _ := out["outputs"].([]any)
	if code != http.StatusOK || len(outs) != 3 { // progress, description, bad_output
		t.Fatalf("ports: %d %v", code, out)
	}
	if code, _ = secretReq(h, http.MethodPost, "/api/script-nodes/ports", json.RawMessage(`{"type":"parse","params":{"schema":[]}}`)); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad params: %d", code)
	}
	if code, _ = secretReq(h, http.MethodPost, "/api/script-nodes/ports", json.RawMessage(`{"type":"nope"}`)); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown type: %d", code)
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// secretReqList is a GET for an endpoint that answers with a JSON array.
func secretReqList(t *testing.T, h http.Handler, path string) (int, []map[string]any) {
	code, body := apiCall(t, h, http.MethodGet, path, nil)
	var out []map[string]any
	_ = json.Unmarshal([]byte(body), &out)
	return code, out
}

func TestBindingAScriptToAStepAndANote(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	qid := seedQuest(t, s)
	var stepID int64
	if err := s.Store.DB.QueryRow(`SELECT id FROM queststep WHERE quest_id = ?`, qid).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	stepGraph := strings.ReplaceAll(scriptBody, "source.note", "source.step")
	stepGraph = strings.Replace(stepGraph, `{"id":4,"type":"sink.note_verdict"}`, `{"id":4,"type":"sink.step_progress"}`, 1)
	stepGraph = strings.Replace(stepGraph, `"outdated"`, `"progress"`, 3)
	stepGraph = strings.Replace(stepGraph, `"type":"bool"`, `"type":"int"`, 1)
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}

	code, step := call(http.MethodPost, "/api/scripts", `{"title":"для шага","graph":`+stepGraph+`}`)
	if code != http.StatusCreated {
		t.Fatalf("a step script: %d %v", code, step)
	}
	code, note := call(http.MethodPost, "/api/scripts", `{"title":"для заметки","graph":`+scriptBody+`}`)
	if code != http.StatusCreated {
		t.Fatalf("a note script: %d %v", code, note)
	}
	stepScript, noteScript := jsonNum(step["id"]), jsonNum(note["id"])
	stepURL := "/api/steps/" + strconv.FormatInt(stepID, 10) + "/script"

	// Mismatches and missing things are told apart.
	if code, _ = call(http.MethodPut, stepURL, `{"script_id":`+noteScript+`}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("a note script on a step: %d", code)
	}
	if code, _ = call(http.MethodPut, stepURL, `{"script_id":9999}`); code != http.StatusNotFound {
		t.Fatalf("an unknown script: %d", code)
	}
	if code, _ = call(http.MethodPut, "/api/steps/9999/script", `{"script_id":`+stepScript+`}`); code != http.StatusNotFound {
		t.Fatalf("an unknown step: %d", code)
	}

	if code, _ = call(http.MethodPut, stepURL, `{"script_id":`+stepScript+`}`); code != http.StatusNoContent {
		t.Fatalf("bind: %d", code)
	}
	_, quest := call(http.MethodGet, "/api/quests/"+strconv.FormatInt(qid, 10), "")
	first := quest["steps"].([]any)[0].(map[string]any)
	if jsonNum(first["script_id"]) != stepScript || first["check_command"] != nil {
		t.Fatalf("the step shows its script and no command: %v", first)
	}

	// A script in use is not deleted, and the answer says where it is used.
	code, out := call(http.MethodDelete, "/api/scripts/"+stepScript, "")
	if code != http.StatusConflict || len(out["uses"].([]any)) != 1 {
		t.Fatalf("delete a bound script: %d %v", code, out)
	}
	code, uses := secretReqList(t, h, "/api/scripts/"+stepScript+"/uses")
	if code != http.StatusOK || len(uses) != 1 || uses[0]["subject_type"] != "step" {
		t.Fatalf("uses: %d %v", code, uses)
	}

	// A command typed over the script replaces it.
	code, _ = call(http.MethodPatch, "/api/quests/"+strconv.FormatInt(qid, 10)+"/steps/"+strconv.FormatInt(stepID, 10), `{"check_command":"echo 1","run_on":"pc"}`)
	if code != http.StatusOK {
		t.Fatalf("patch: %d", code)
	}
	if code, _ = secretReqList(t, h, "/api/scripts/"+stepScript+"/uses"); code != http.StatusOK {
		t.Fatal(code)
	}
	if _, uses = secretReqList(t, h, "/api/scripts/"+stepScript+"/uses"); len(uses) != 0 {
		t.Fatalf("the binding must be gone: %v", uses)
	}

	// Notes take their own scripts the same way; detaching is idempotent.
	noteRow, err := s.Store.Knowledge.CreateNote(t.Context(), knowledge.Note{Title: "n"})
	if err != nil {
		t.Fatal(err)
	}
	noteURL := "/api/notes/" + strconv.FormatInt(noteRow.ID, 10) + "/script"
	if code, _ = call(http.MethodPut, noteURL, `{"script_id":`+noteScript+`}`); code != http.StatusNoContent {
		t.Fatalf("bind a note: %d", code)
	}
	for i := 0; i < 2; i++ {
		if code, _ = call(http.MethodDelete, noteURL, ""); code != http.StatusNoContent {
			t.Fatalf("unbind: %d", code)
		}
	}
	if code, _ = call(http.MethodDelete, "/api/scripts/"+noteScript, ""); code != http.StatusNoContent {
		t.Fatalf("a free script can be deleted: %d", code)
	}
}

func TestCustomNodesOverHTTP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}
	def := `{"title":"Число","inputs":[{"name":"secrets","node":1,"port":"secrets"}],
		"outputs":[{"name":"out","node":1,"port":"stdout"}],
		"fields":[{"name":"cmd","type":"str","default":"echo 1","node":1,"param":"text"}],
		"graph":{"nodes":[{"id":1,"type":"code","params":{"text":"echo 1"}}],"edges":[]}}`

	code, out := call(http.MethodPost, "/api/script-custom-nodes", def)
	if code != http.StatusCreated || out["version"] != float64(1) || out["needs_runner"] != true {
		t.Fatalf("create: %d %v", code, out)
	}
	id := jsonNum(out["id"])
	path := "/api/script-custom-nodes/" + id

	if code, out = call(http.MethodPost, "/api/script-custom-nodes", `{"title":"","graph":{"nodes":[],"edges":[]}}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("a bad definition: %d %v", code, out)
	}
	if code, list := secretReqList(t, h, "/api/script-custom-nodes"); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: %d %v", code, list)
	}
	if code, out = call(http.MethodGet, path, ""); code != http.StatusOK || out["title"] != "Число" {
		t.Fatalf("get: %d %v", code, out)
	}
	if code, _ = call(http.MethodGet, "/api/script-custom-nodes/999", ""); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}

	// The catalogue and the port resolver know the node, the validator accepts a script that uses it.
	_, nodes := secretReqList(t, h, "/api/script-nodes")
	found := false
	for _, n := range nodes {
		found = found || n["name"] == "custom."+id
	}
	if !found {
		t.Fatalf("custom node missing from the catalogue: %v", nodes)
	}
	if code, out = call(http.MethodPost, "/api/script-nodes/ports", `{"type":"custom.`+id+`","params":{"cmd":"echo 5"}}`); code != http.StatusOK {
		t.Fatalf("ports: %d %v", code, out)
	}
	if code, out = call(http.MethodPost, "/api/script-nodes/ports", `{"type":"custom.`+id+`","params":{"nope":1}}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("ports of a wrong field: %d %v", code, out)
	}
	use := `{"nodes":[{"id":1,"type":"source.step"},{"id":2,"type":"custom.` + id + `"},{"id":3,"type":"sink.done"}],
		"edges":[{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},{"from":{"node":2,"port":"out"},"to":{"node":3,"port":"trigger"}}]}`
	if code, out = call(http.MethodPost, "/api/scripts/validate", `{"graph":`+use+`}`); code != http.StatusOK || out["ok"] != true || out["needs_runner"] != true {
		t.Fatalf("validate: %d %v", code, out)
	}
	code, out = call(http.MethodPost, "/api/scripts", `{"title":"со своей нодой","purpose":"step","graph":`+use+`}`)
	if code != http.StatusCreated || out["purpose"] != "step" {
		t.Fatalf("script: %d %v", code, out)
	}
	scriptPath := "/api/scripts/" + jsonNum(out["id"])

	// The starting library installs once.
	if code, out = call(http.MethodPost, "/api/script-custom-nodes/starters", `{}`); code != http.StatusOK || len(out["created"].([]any)) == 0 {
		t.Fatalf("starters: %d %v", code, out)
	}
	if code, out = call(http.MethodPost, "/api/script-custom-nodes/starters", `{}`); code != http.StatusOK || len(out["created"].([]any)) != 0 {
		t.Fatalf("starters again: %d %v", code, out)
	}

	// Stale version, in use, then free to delete.
	if code, _ = call(http.MethodPut, path, strings.Replace(def, `{"title"`, `{"version":7,"title"`, 1)); code != http.StatusConflict {
		t.Fatalf("stale: %d, want 409", code)
	}
	if code, out = call(http.MethodPut, path, strings.Replace(def, `{"title":"Число"`, `{"version":1,"title":"Число 2"`, 1)); code != http.StatusOK || out["version"] != float64(2) {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out = call(http.MethodDelete, path, ""); code != http.StatusConflict || out["used_by"] == nil {
		t.Fatalf("delete while used: %d %v", code, out)
	}
	if code, _ = call(http.MethodDelete, scriptPath, ""); code != http.StatusNoContent {
		t.Fatalf("delete script: %d", code)
	}
	if code, _ = call(http.MethodDelete, path, ""); code != http.StatusNoContent {
		t.Fatalf("delete node: %d", code)
	}
}

// A test emit of a template that has neither script nor command has nothing to run; a manual emit of it still makes the
// quest from its own steps.
func TestTemplateEmitTestNeedsAScriptOrCommand(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	code, out := secretReq(h, http.MethodPost, "/api/templates", map[string]any{"title": "Обычный", "enabled": false, "steps": []any{map[string]any{"title": "Шаг"}}})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	id := jsonNum(out["id"])
	if code, out = secretReq(h, http.MethodPost, "/api/templates/"+id+"/emit-test", map[string]any{}); code != http.StatusUnprocessableEntity {
		t.Fatalf("emit-test of a plain template: %d %v", code, out)
	}
	if code, out = secretReq(h, http.MethodPost, "/api/templates/"+id+"/emit", map[string]any{}); code != http.StatusCreated || out["title"] != "Обычный" {
		t.Fatalf("manual emit of a plain template: %d %v", code, out)
	}
	if code, _ = secretReq(h, http.MethodPost, "/api/templates/9999/emit-test", map[string]any{}); code != http.StatusNotFound {
		t.Fatalf("emit-test of a missing template: %d", code)
	}
}
