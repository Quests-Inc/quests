package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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

	code, nodes := secretReqList(t, h, "/api/script-nodes")
	if code != http.StatusOK || len(nodes) != 16 || nodes[0]["kind"] != "source" {
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
