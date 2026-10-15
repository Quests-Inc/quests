package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/mcp"
)

// mcpCall drives the MCP server against the real API handler and returns the decoded tool result.
type mcpSession struct {
	t   *testing.T
	srv *mcp.Server
	n   int
}

func (m *mcpSession) call(name string, args any) (map[string]any, string, bool) {
	m.t.Helper()
	m.n++
	a, _ := json.Marshal(args)
	req := `{"jsonrpc":"2.0","id":` + string(rune('0'+m.n%10)) + `,"method":"tools/call","params":{"name":"` + name + `","arguments":` + string(a) + `}}` + "\n"
	var out bytes.Buffer
	if err := m.srv.Serve(context.Background(), strings.NewReader(req), &out); err != nil {
		m.t.Fatal(err)
	}
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		m.t.Fatalf("%s: %v: %s", name, err, out.String())
	}
	text := resp.Result.Content[0].Text
	var obj map[string]any
	_ = json.Unmarshal([]byte(text), &obj)
	return obj, text, resp.Result.IsError
}

func TestMCPAgainstRealAPI(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	m := &mcpSession{t: t, srv: mcp.New(mcp.NewClient(ts.URL, ""), "test")}

	// workspaces come from the API
	ws, text, isErr := m.call("list_workspaces", map[string]any{})
	if isErr || ws["current"] == "" {
		t.Fatalf("list_workspaces: %s", text)
	}

	// a quest with steps, then the compact list and the detail
	created, text, isErr := m.call("create_quest", map[string]any{
		"title": "MCP e2e", "description": strings.Repeat("d", 6000), "significance": "epic",
		"steps": []map[string]any{{"title": "first", "description": "step body"}, {"title": "second"}},
	})
	if isErr {
		t.Fatalf("create_quest: %s", text)
	}
	qid := int64(created["id"].(float64))
	if created["ok"] != true || created["progress_label"] != "0 / 2" {
		t.Fatalf("create result: %v", created)
	}

	list, text, isErr := m.call("list_quests", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	items := list["items"].([]any)
	if len(items) != 1 || list["total"].(float64) != 1 {
		t.Fatalf("list: %s", text)
	}
	if strings.Contains(text, "step body") || strings.Contains(text, "dddd") {
		t.Fatalf("list must not carry bodies: %s", text)
	}
	item := items[0].(map[string]any)
	if item["description_len"].(float64) != 6000 || item["next_step"].(map[string]any)["title"] != "first" {
		t.Fatalf("summary item: %v", item)
	}

	got, text, isErr := m.call("get_quest", map[string]any{"id": qid})
	if isErr || got["description_truncated"] != true || len(got["steps"].([]any)) != 2 {
		t.Fatalf("get_quest: %s", text)
	}
	stepID := int64(got["steps"].([]any)[0].(map[string]any)["id"].(float64))

	// several step changes in one call, closing the first step
	batch, text, isErr := m.call("update_steps", map[string]any{"quest_id": qid, "ops": []map[string]any{
		{"op": "update", "step_id": stepID, "progress_current": 1},
		{"op": "add", "title": "third"},
	}})
	if isErr || batch["progress_label"] != "1 / 3" {
		t.Fatalf("update_steps: %s", text)
	}

	step, text, isErr := m.call("get_step", map[string]any{"id": stepID, "quest_id": qid})
	if isErr || step["step"].(map[string]any)["description"] != "step body" {
		t.Fatalf("get_step: %s", text)
	}

	// finishing it moves it out of the default (live) list into history
	if _, text, isErr = m.call("update_quest", map[string]any{"id": qid, "status": "completed"}); isErr {
		t.Fatal(text)
	}
	if list, _, _ = m.call("list_quests", map[string]any{}); list["total"].(float64) != 0 {
		t.Fatalf("completed quest still in the live list: %v", list)
	}
	if list, _, _ = m.call("list_quests", map[string]any{"status": "completed"}); list["total"].(float64) != 1 {
		t.Fatalf("history list: %v", list)
	}

	// notes: category guard, brief list, cut text, flags
	cat, text, isErr := m.call("create_note", map[string]any{"title": "Cat", "is_category": true, "description": "what goes here"})
	if isErr {
		t.Fatal(text)
	}
	if _, text, isErr = m.call("create_note", map[string]any{"title": "loose"}); !isErr || !strings.Contains(text, "CATEGORY") {
		t.Fatalf("expected the category refusal, got %q", text)
	}
	note, text, isErr := m.call("create_note", map[string]any{"title": "Child", "parent_id": int64(cat["id"].(float64)), "description": strings.Repeat("n", 5000)})
	if isErr {
		t.Fatal(text)
	}
	nid := int64(note["id"].(float64))
	nl, text, _ := m.call("list_notes", map[string]any{"category_id": int64(cat["id"].(float64))})
	if nl["total"].(float64) != 1 || strings.Contains(text, "nnnn") {
		t.Fatalf("list_notes: %s", text)
	}
	if _, text, isErr = m.call("set_note_flag", map[string]any{"id": nid, "flag": "outdated", "reason": "old"}); isErr {
		t.Fatal(text)
	}
	if nl, text, _ = m.call("list_notes", map[string]any{"flags": "-outdated", "category_id": int64(cat["id"].(float64))}); nl["total"].(float64) != 0 {
		t.Fatalf("outdated note must be filtered out: %s", text)
	}
	gn, text, isErr := m.call("get_note", map[string]any{"id": nid, "max_chars": 100})
	if isErr || gn["description_len"].(float64) != 5000 || gn["description_truncated"] != true {
		t.Fatalf("get_note: %s", text)
	}

	// a missing thing is an error result with the code first
	if _, text, isErr = m.call("get_quest", map[string]any{"id": 987654}); !isErr || !strings.HasPrefix(text, "404:") {
		t.Fatalf("missing quest: %q", text)
	}
}
