package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
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

func TestHookAreaAndLimitsOverHTTP(t *testing.T) {
	h, s := secretsServer(t)
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}
	ctx := wsctx.With(t.Context(), 1)
	note, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "ветка"})
	if err != nil {
		t.Fatal(err)
	}
	nid := strconv.FormatInt(note.ID, 10)

	code, ev := call(http.MethodGet, "/api/hook-events", "")
	events, _ := ev["events"].([]any)
	if code != http.StatusOK || len(events) < 10 {
		t.Fatalf("the catalog lists the fields of each event: %d %v", code, ev)
	}

	code, sc := call(http.MethodPost, "/api/scripts", `{"title":"хук-скрипт","graph":`+hookScriptBody+`}`)
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	sid := jsonNum(sc["id"])

	code, out := call(http.MethodPost, "/api/hooks", `{"events":["note"],"script_id":`+sid+`,"run_on":"home","scope_kind":"note_tree","scope_id":`+nid+`,"cooldown_sec":60,"max_per_hour":10,"include_private":true}`)
	if code != http.StatusCreated || out["scope_kind"] != "note_tree" || out["cooldown_sec"] != float64(60) || out["max_per_hour"] != float64(10) || out["include_private"] != true {
		t.Fatalf("create with an area: %d %v", code, out)
	}
	if got := out["events"].([]any); len(got) != 3 || got[0] != "note_created" {
		t.Fatalf("the alias expands to the note events: %v", got)
	}
	hid := jsonNum(out["id"])

	// An area needs an entity that exists; a limit has a range.
	for _, bad := range []string{
		`{"scope_kind":"note"}`, `{"scope_kind":"note","scope_id":999999}`, `{"scope_kind":"everything"}`, `{"max_per_hour":0}`, `{"cooldown_sec":-1}`,
	} {
		if code, _ = call(http.MethodPatch, "/api/hooks/"+hid, bad); code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", bad, code)
		}
	}
	// The old way still works: a quest_id is a quest area, and null puts the hook back on the whole workspace.
	qid := strconv.FormatInt(pgtest.SeedQuest(t, s.Store.DB, "цель"), 10)
	code, out = call(http.MethodPatch, "/api/hooks/"+hid, `{"quest_id":`+qid+`}`)
	if code != http.StatusOK || out["scope_kind"] != "quest" || out["scope_id"] == nil {
		t.Fatalf("quest_id as an area: %d %v", code, out)
	}
	code, out = call(http.MethodPatch, "/api/hooks/"+hid, `{"quest_id":null}`)
	if code != http.StatusOK || out["scope_kind"] != "workspace" || out["scope_id"] != nil {
		t.Fatalf("back to the workspace: %d %v", code, out)
	}
}
