package httpapi

import (
	"fmt"
	"github.com/quests-inc/quests/go/internal/mcp"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPCalendarDeadlineAndSearch(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	m := &mcpSession{t: t, srv: mcp.New(mcp.NewClient(ts.URL, ""), "test")}
	call := func(name string, args any) map[string]any {
		t.Helper()
		obj, text, bad := m.call(name, args)
		if bad {
			t.Fatalf("%s: %s", name, text)
		}
		return obj
	}
	event := call("event", map[string]any{"action": "create", "title": "Конференция", "starts_at": "2030-05-10T10:00:00+03:00", "ends_at": "2030-05-10T12:00:00+03:00", "description": strings.Repeat("a", 5000)})
	id := event["id"]
	listed := call("event", map[string]any{"action": "list", "q": fmt.Sprintf("event=%.0f", id.(float64))})
	if len(listed["items"].([]any)) != 1 {
		t.Fatalf("event search: %v", listed)
	}
	detail := call("event", map[string]any{"action": "get", "id": id, "max_chars": 20})
	if detail["description_truncated"] != true || detail["starts_at"] != "2030-05-10T07:00:00.000000Z" {
		t.Fatalf("get/timezone: %v", detail)
	}
	created := call("create_quest", map[string]any{"title": "Доклад", "questline": "none", "event_id": id, "event_edge": "start", "event_dir": "before", "event_offset_seconds": 3600, "duration_seconds": 7200, "expire_on_deadline": false})
	qid := created["id"]
	q := call("get_quest", map[string]any{"id": qid})
	if q["event_id"] != id || q["event_title"] != "Конференция" || q["duration_seconds"] != float64(7200) || q["expire_on_deadline"] != false {
		t.Fatalf("binding hidden or options lost: %v", q)
	}
	call("event", map[string]any{"action": "update", "id": id, "starts_at": "2030-05-11T07:00:00Z", "ends_at": "2030-05-11T09:00:00Z"})
	q = call("get_quest", map[string]any{"id": qid})
	if !strings.Contains(q["deadline_at"].(string), "2030-05-11T06:00:00") || q["duration_seconds"] != float64(7200) {
		t.Fatalf("moving event lost live link/window: %v", q)
	}
	call("update_quest", map[string]any{"id": qid, "event_offset_seconds": 1800, "expire_on_deadline": true})
	q = call("get_quest", map[string]any{"id": qid})
	if q["event_offset_seconds"] != float64(1800) || q["expire_on_deadline"] != true {
		t.Fatalf("partial binding/policy: %v", q)
	}
	found := call("search_entities", map[string]any{"q": fmt.Sprintf("quest=%.0f", qid.(float64)), "types": []string{"quest", "note"}})
	if len(found["hits"].([]any)) != 1 {
		t.Fatalf("shared search: %v", found)
	}
	call("icon", map[string]any{"action": "color", "event": id, "color": "c47a20"})
	detail = call("event", map[string]any{"action": "get", "id": id, "max_chars": 1})
	if detail["color"] != "#c47a20" || detail["title"] != "Конференция" {
		t.Fatalf("partial color update discarded event fields: %v", detail)
	}
	call("event", map[string]any{"action": "delete", "id": id})
	q = call("get_quest", map[string]any{"id": qid})
	if q["event_id"] != nil || q["deadline_at"] == nil || q["duration_seconds"] != float64(7200) || q["expire_on_deadline"] != true {
		t.Fatalf("event deletion lost resolved options: %v", q)
	}
	call("update_quest", map[string]any{"id": qid, "clear_deadline": true})
	q = call("get_quest", map[string]any{"id": qid})
	if q["deadline_at"] != nil || q["event_id"] != nil {
		t.Fatalf("clear deadline: %v", q)
	}
}
