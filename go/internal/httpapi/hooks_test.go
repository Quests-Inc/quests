package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

const hookScriptBody = `{"nodes":[
	{"id":1,"type":"source.quest"},
	{"id":2,"type":"code","params":{"text":"echo ok"}},
	{"id":3,"type":"sink.done"}],
 "edges":[
	{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
	{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"trigger"}}]}`

func TestHooksOverHTTP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}

	code, ev := call(http.MethodGet, "/api/hook-events", "")
	if code != http.StatusOK || ev["aliases"] == nil || ev["kinds"] == nil {
		t.Fatalf("events: %d %v", code, ev)
	}

	code, sc := call(http.MethodPost, "/api/scripts", `{"title":"хук-скрипт","graph":`+hookScriptBody+`}`)
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	sid := jsonNum(sc["id"])

	code, out := call(http.MethodPost, "/api/hooks", `{"name":"на complete","events":["complete"],"script_id":`+sid+`,"run_on":"home"}`)
	if code != http.StatusCreated || out["events"].([]any)[0] != "quest_completed" {
		t.Fatalf("create: %d %v", code, out)
	}
	hid := jsonNum(out["id"])

	code, list := secretReqList(t, h, "/api/hooks")
	if code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: %d %v", code, list)
	}

	code, out = call(http.MethodPatch, "/api/hooks/"+hid, `{"enabled":false}`)
	if code != http.StatusOK || out["enabled"] != false {
		t.Fatalf("patch: %d %v", code, out)
	}

	if code, _ = call(http.MethodPost, "/api/hooks", `{"events":["complete"],"script_id":`+sid+`}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("no runner: %d", code)
	}

	if code, _ = call(http.MethodDelete, "/api/hooks/"+hid, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = call(http.MethodGet, "/api/hooks/"+hid, ""); code != http.StatusNotFound {
		t.Fatalf("gone: %d", code)
	}
}
