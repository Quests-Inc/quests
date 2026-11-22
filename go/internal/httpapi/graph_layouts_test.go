package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// Saved map layouts: create with a title and the page's state, list without the state, read it back, rename, replace, delete.
func TestGraphLayoutsOverHTTP(t *testing.T) {
	h, _ := secretsServer(t)
	call := func(method, path, body string) (int, map[string]any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}
	state := `{"center":{"type":"quest","id":3},"depth":2,"positions":{"quest:3":{"x":10,"y":20}}}`
	code, l := call(http.MethodPost, "/api/graph-layouts", `{"title":"  Работа  ","state":`+state+`}`)
	if code != http.StatusCreated || l["title"] != "Работа" {
		t.Fatalf("create: %d %v", code, l)
	}
	id := jsonNum(l["id"])

	code, list := secretReqList(t, h, "/api/graph-layouts")
	if code != http.StatusOK || len(list) != 1 || list[0]["state"] != nil {
		t.Fatalf("the list has titles, not states: %d %v", code, list)
	}
	code, l = call(http.MethodGet, "/api/graph-layouts/"+id, "")
	st, _ := l["state"].(map[string]any)
	if code != http.StatusOK || st["depth"] != float64(2) {
		t.Fatalf("read back: %d %v", code, l)
	}
	code, l = call(http.MethodPatch, "/api/graph-layouts/"+id, `{"title":"Дом"}`)
	st, _ = l["state"].(map[string]any)
	if code != http.StatusOK || l["title"] != "Дом" || st["depth"] != float64(2) {
		t.Fatalf("a rename keeps the state: %d %v", code, l)
	}
	code, l = call(http.MethodPatch, "/api/graph-layouts/"+id, `{"state":{"depth":4}}`)
	st, _ = l["state"].(map[string]any)
	if code != http.StatusOK || l["title"] != "Дом" || st["depth"] != float64(4) {
		t.Fatalf("a new state keeps the title: %d %v", code, l)
	}
	for _, bad := range []string{`{"title":"","state":{}}`, `{"title":"x","state":[1]}`, `{"title":"x"}`} {
		if code, out := call(http.MethodPost, "/api/graph-layouts", bad); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %v", bad, code, out)
		}
	}
	if code, _ = call(http.MethodDelete, "/api/graph-layouts/"+id, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = call(http.MethodGet, "/api/graph-layouts/"+id, ""); code != http.StatusNotFound {
		t.Fatalf("gone: %d", code)
	}
}

// Creating, renaming, replacing and deleting a layout tells the author's open pages. The state itself stays out of the event.
func TestGraphLayoutPublishesToTheAuthor(t *testing.T) {
	h, s := secretsServer(t)
	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		return secretReq(h, method, path, in)
	}
	code, l := call(http.MethodPost, "/api/graph-layouts", `{"title":"Сессия 2","state":{"depth":1,"positions":{"note:1":{"x":1}}}}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, l)
	}
	id := jsonNum(l["id"])
	want := func(kind string, state bool) {
		t.Helper()
		var got map[string]any
		for _, e := range s.Hub.EventsSince(0, 0) {
			if e["kind"] == kind && jsonNum(e["layout_id"]) == id {
				got = e
			}
		}
		if got == nil {
			t.Fatalf("no %s in %v", kind, s.Hub.EventsSince(0, 0))
		}
		if kind != "graph_layout_deleted" && got["title"] == "" {
			t.Fatalf("%s has no title: %v", kind, got)
		}
		if _, leaked := got["positions"]; leaked {
			t.Fatalf("state leaked in %s: %v", kind, got)
		}
		if state && got["state"] != true {
			t.Fatalf("%s should name a state change: %v", kind, got)
		}
		if !state && got["state"] != nil {
			t.Fatalf("%s should not name a state change: %v", kind, got)
		}
	}
	want("graph_layout_created", true)
	if code, l = call(http.MethodPatch, "/api/graph-layouts/"+id, `{"title":"Дом"}`); code != http.StatusOK {
		t.Fatalf("rename: %d %v", code, l)
	}
	want("graph_layout_updated", false)
	if code, _ = call(http.MethodPatch, "/api/graph-layouts/"+id, `{"state":{"depth":3}}`); code != http.StatusOK {
		t.Fatalf("state: %d", code)
	}
	want("graph_layout_updated", true)
	if code, _ = call(http.MethodDelete, "/api/graph-layouts/"+id, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	want("graph_layout_deleted", false)
}

// A layout is its author's: another user of the workspace neither lists, reads, changes nor deletes it.
func TestGraphLayoutsArePersonal(t *testing.T) {
	_, s := secretsServer(t)
	var alice, bob int64
	for _, u := range []struct {
		name string
		id   *int64
	}{{"alice", &alice}, {"bob", &bob}} {
		if err := s.Store.DB.QueryRow(`INSERT INTO appuser (username, password_hash, created_at) VALUES (?, 'x', now()) RETURNING id`, u.name).Scan(u.id); err != nil {
			t.Fatal(err)
		}
	}
	as := func(user int64) context.Context { return wsctx.WithUser(wsctx.With(context.Background(), 1), user) }
	l, err := s.Store.CreateGraphLayout(as(alice), "моё", json.RawMessage(`{"depth":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Store.ListGraphLayouts(as(bob)); len(list) != 0 {
		t.Fatalf("bob sees alice's layouts: %v", list)
	}
	if _, err := s.Store.GetGraphLayout(as(bob), l.ID); err == nil {
		t.Fatal("bob read alice's layout")
	}
	if err := s.Store.DeleteGraphLayout(as(bob), l.ID); err == nil {
		t.Fatal("bob deleted alice's layout")
	}
	if list, _ := s.Store.ListGraphLayouts(as(alice)); len(list) != 1 {
		t.Fatalf("alice lost her layout: %v", list)
	}
}
