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
