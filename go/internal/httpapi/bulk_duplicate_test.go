package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestBulkValuesAndAtomicNoteDuplicate(t *testing.T) {
	h, s := secretsServer(t)
	ctx := wsctx.With(t.Context(), 1)
	source, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Character", Description: "{{mod}} ![image](attachment=99)", Pinned: true, IsCategory: true})
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/notes/%d", source.ID)
	input := map[string]any{"values": []any{map[string]any{"name": "mod", "type": "int", "formula": "(stat-10)/2"}, map[string]any{"name": "stat", "type": "int", "value": 14}, map[string]any{"name": "alive", "type": "bool", "value": false}}}
	if code, out := secretReq(h, http.MethodPut, base+"/values", input); code != 200 {
		t.Fatal(code, out)
	}
	code, out := secretReq(h, http.MethodPost, base+"/duplicate", map[string]any{"title": "Copy", "parent_id": nil})
	if code != 201 {
		t.Fatal(code, out)
	}
	if out["title"] != "Copy" || out["pinned"] != false || out["is_category"] != false || out["parent_id"] != nil {
		t.Fatal(out)
	}
	copyURL := "/api/notes/" + jsonNum(out["id"])
	code, values := secretReq(h, http.MethodGet, copyURL+"/values", nil)
	if code != 200 || len(values["own"].([]any)) != 3 {
		t.Fatal(code, values)
	}
	if code, _ := secretReq(h, http.MethodPut, copyURL+"/values/stat", map[string]any{"type": "int", "value": 20}); code != 200 {
		t.Fatal(code)
	}
	_, values = secretReq(h, http.MethodGet, base+"/values", nil)
	for _, row := range values["effective"].([]any) {
		v := row.(map[string]any)
		if v["name"] == "mod" && v["value"] != float64(2) {
			t.Fatal("source modified", v)
		}
	}
	if _, err := s.Store.Knowledge.UpdateNote(ctx, source.ID, map[string]any{"is_private": true}); err != nil {
		t.Fatal(err)
	}
	for _, op := range []struct{ method, path, body string }{{"POST", base + "/duplicate", `{}`}, {"PUT", base + "/values", `{"values":[{"name":"secret","type":"int","value":1}]}`}} {
		r := httptest.NewRequest(op.method, op.path, bytes.NewBufferString(op.body))
		r.Header.Set("X-Quests-Source", "mcp")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("private: %d %s", w.Code, w.Body.String())
		}
	}
	// A failing duplicate never leaves an extra note behind.
	before, _ := s.Store.Knowledge.ListNotes(ctx, knowledge.NoteFilter{})
	code, _ = secretReq(h, http.MethodPost, base+"/duplicate", json.RawMessage(`{"title":"bad","parent_id":999999999}`))
	if code < 400 {
		t.Fatal("bad parent accepted")
	}
	after, _ := s.Store.Knowledge.ListNotes(ctx, knowledge.NoteFilter{})
	if len(before) != len(after) {
		t.Fatal("partial copy")
	}
}

func TestSampleFlagAndCreatingIndependentNote(t *testing.T) {
	h, _ := secretsServer(t)
	code, source := secretReq(h, "POST", "/api/notes", map[string]any{"title": "Blueprint", "is_sample": true, "description": "Purpose"})
	if code != 201 || source["is_sample"] != true {
		t.Fatal(code, source)
	}
	base := "/api/notes/" + jsonNum(source["id"])
	code, copy := secretReq(h, "POST", base+"/duplicate", map[string]any{"title": "New card", "parent_id": nil})
	if code != 201 || copy["is_sample"] != false {
		t.Fatal(code, copy)
	}
	code, list := secretReqList(t, h, "/api/notes?flags=sample")
	if code != 200 || len(list) != 1 || list[0]["title"] != "Blueprint" {
		t.Fatal(code, list)
	}
}

func TestSampleIsReadOnlyAcrossNotesEnvironmentAndMap(t *testing.T) {
	h, s := secretsServer(t)
	code, source := secretReq(h, "POST", "/api/notes", map[string]any{"title": "Locked blueprint", "description": "Original"})
	if code != 201 {
		t.Fatal(code, source)
	}
	id := jsonNum(source["id"])
	base := "/api/notes/" + id
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{"PUT", base + "/values/hp", map[string]any{"type": "int", "value": 12}},
		{"PUT", base + "/secrets/TOKEN", map[string]any{"value": "hidden"}},
		{"PATCH", base, map[string]any{"is_sample": true}},
	} {
		if status, out := secretReq(h, request.method, request.path, request.body); status >= 300 {
			t.Fatal(status, out)
		}
	}
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", base, map[string]any{"description": "Changed"}},
		{"PATCH", base, map[string]any{"description_insert": map[string]any{"text": "changed", "position": 0}}},
		{"PATCH", base, map[string]any{"title": "Changed"}},
		{"PATCH", base, map[string]any{"is_sample": false, "description": "Sneak edit"}},
		{"PUT", base + "/values/hp", map[string]any{"type": "int", "value": 99}},
		{"PUT", base + "/values", map[string]any{"values": []any{map[string]any{"name": "hp", "type": "int", "value": 99}}}},
		{"DELETE", base + "/values/hp", nil},
		{"PUT", base + "/secrets/TOKEN", map[string]any{"value": "replaced"}},
		{"DELETE", base + "/secrets/TOKEN", nil},
		{"POST", "/api/graph-draft/ops", map[string]any{"kind": "add_ref", "payload": map[string]any{"from": "note:" + id, "to": "questline:7"}}},
	} {
		if status, out := secretReq(h, request.method, request.path, request.body); status != 403 {
			t.Fatalf("%s %s: %d %v", request.method, request.path, status, out)
		}
	}
	ctx := wsctx.With(t.Context(), 1)
	// Direct storage writes, including scripts, cannot bypass the HTTP policy.
	if _, err := s.Store.DB.ExecContext(ctx, "UPDATE named_value SET value='99' WHERE note_id=?", source["id"]); err == nil {
		t.Fatal("direct value update accepted")
	}
	if _, err := s.Store.DB.ExecContext(ctx, "DELETE FROM secret WHERE note_id=?", source["id"]); err == nil {
		t.Fatal("direct secret deletion accepted")
	}
	if status, out := secretReq(h, "POST", base+"/duplicate", map[string]any{"title": "Editable copy"}); status != 201 {
		t.Fatal(status, out)
	}
	if status, out := secretReq(h, "PATCH", base, map[string]any{"is_sample": false}); status != 200 {
		t.Fatal(status, out)
	}
	if status, out := secretReq(h, "PATCH", base, map[string]any{"description": "Now editable"}); status != 200 {
		t.Fatal(status, out)
	}
}
