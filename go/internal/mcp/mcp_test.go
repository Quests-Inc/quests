package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI records requests and answers from a route table.
type fakeAPI struct {
	mu     sync.Mutex
	seen   []string
	hdrs   []http.Header
	routes map[string]string // "METHOD /path" → JSON body (status 200)
	fail   map[string]int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	key := r.Method + " " + r.URL.Path
	f.seen = append(f.seen, key+"?"+r.URL.RawQuery)
	f.hdrs = append(f.hdrs, r.Header.Clone())
	f.mu.Unlock()
	if code, ok := f.fail[key]; ok {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, `{"detail":"nope"}`)
		return
	}
	body, ok := f.routes[key]
	if !ok {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"detail":"no route `+key+`"}`)
		return
	}
	_, _ = io.WriteString(w, body)
}

func newTestServer(t *testing.T, f *fakeAPI) (*Server, *Client) {
	t.Helper()
	if f.routes == nil {
		f.routes = map[string]string{}
	}
	if _, ok := f.routes["GET /api/workspaces"]; !ok {
		f.routes["GET /api/workspaces"] = `{"current":1,"workspaces":[{"id":1,"name":"Основное"},{"id":2,"name":"Тестовое"}]}`
	}
	ts := httptest.NewServer(f)
	t.Cleanup(ts.Close)
	c := NewClient(ts.URL, "tok")
	return New(c, "test"), c
}

// rpc sends requests through Serve and returns the decoded responses.
func rpc(t *testing.T, s *Server, reqs ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(reqs, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad response line %q: %v", line, err)
		}
		resps = append(resps, m)
	}
	return resps
}

func call(id int, name, args string) string {
	args = strings.NewReplacer("\n", "", "\t", "").Replace(args) // one message per line
	return `{"jsonrpc":"2.0","id":` + string(rune('0'+id)) + `,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

// result unpacks a tools/call response: the text, and whether it is an error.
func result(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	res, _ := resp["result"].(map[string]any)
	if res == nil {
		t.Fatalf("no result: %v", resp)
	}
	content := res["content"].([]any)
	isErr, _ := res["isError"].(bool)
	return content[0].(map[string]any)["text"].(string), isErr
}

func TestProtocolBasics(t *testing.T) {
	s, _ := newTestServer(t, &fakeAPI{})
	resps := rpc(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
		`not json`,
	)
	if len(resps) != 4 { // the notification gets no answer
		t.Fatalf("got %d responses: %v", len(resps), resps)
	}
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" || !strings.Contains(init["instructions"].(string), "Quests") {
		t.Fatalf("initialize: %v", init)
	}
	if resps[2]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("unknown method: %v", resps[2])
	}
	if resps[3]["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Fatalf("parse error: %v", resps[3])
	}
}

func TestToolsListShape(t *testing.T) {
	s, _ := newTestServer(t, &fakeAPI{})
	resps := rpc(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := resps[0]["result"].(map[string]any)["tools"].([]any)
	// A budget of our own, not a protocol limit: the agent loads the tool list lazily, so the cost is the size of the schemas.
	if len(tools) < 40 || len(tools) > 70 {
		t.Fatalf("tool count %d outside the budget of 40..70", len(tools))
	}
	byName := map[string]map[string]any{}
	size := 0
	for _, raw := range tools {
		tl := raw.(map[string]any)
		name := tl["name"].(string)
		byName[name] = tl
		if tl["description"].(string) == "" {
			t.Errorf("%s: empty description", name)
		}
		schema := tl["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("%s: schema %v", name, schema)
		}
		b, _ := json.Marshal(tl)
		size += len(b)
	}
	for _, n := range []string{"list_quests", "get_quest", "get_step", "update_steps", "set_note_flag", "list_workspaces", "switch_workspace",
		"icon", "get_attachment", "set_tags", "list_secrets", "dry_run_emit_pool", "script", "hook"} {
		if byName[n] == nil {
			t.Errorf("missing tool %s", n)
		}
	}
	for _, gone := range []string{"get_active_context", "get_inactive_context", "get_base_context", "set_note_outdated", "set_quest_tags", "list_emit_attempts"} {
		if byName[gone] != nil {
			t.Errorf("%s must be replaced", gone)
		}
	}
	// the schemas are what sits in the agent's context all the time: keep an eye on their size.
	if size > 60000 {
		t.Errorf("tools/list is %d bytes; the whole point of the rewrite is to stay small", size)
	}
	// embedded structs are flattened (add_step has stepIn's fields at the top level).
	props := byName["add_step"]["inputSchema"].(map[string]any)["properties"].(map[string]any)
	for _, k := range []string{"quest_id", "title", "progress_total", "run_mode"} {
		if props[k] == nil {
			t.Errorf("add_step lacks %s: %v", k, props)
		}
	}
	req := byName["add_step"]["inputSchema"].(map[string]any)["required"].([]any)
	if len(req) != 2 {
		t.Errorf("add_step required %v", req)
	}
}

func TestListQuestsAndWorkspaceHeader(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"GET /api/quests": `{"items":[{"id":7,"title":"x","next_step":{"id":3,"title":"s"}}],"total":1,"truncated":false}`,
	}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "list_quests", `{}`),
		call(2, "switch_workspace", `{"name":"тестовое"}`),
		call(3, "list_quests", `{"status":"completed","limit":500}`),
		call(4, "switch_workspace", `{"name":"нет такого"}`),
	)
	text, isErr := result(t, resps[0])
	if isErr || !strings.Contains(text, `"workspace":"Основное"`) || !strings.Contains(text, `"next_step"`) {
		t.Fatalf("first: %s", text)
	}
	if text, _ := result(t, resps[1]); !strings.Contains(text, `"previous":"Основное"`) || !strings.Contains(text, `"current":"Тестовое"`) {
		t.Fatalf("switch: %s", text)
	}
	if text, _ := result(t, resps[2]); !strings.Contains(text, `"workspace":"Тестовое"`) {
		t.Fatalf("after switch: %s", text)
	}
	if text, isErr := result(t, resps[3]); !isErr || !strings.HasPrefix(text, "404: unknown workspace") {
		t.Fatalf("bad switch: %s", text)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var quests []int
	for i, k := range f.seen {
		if strings.HasPrefix(k, "GET /api/quests?") {
			quests = append(quests, i)
		}
	}
	if len(quests) != 2 {
		t.Fatalf("requests: %v", f.seen)
	}
	first, second := quests[0], quests[1]
	if f.hdrs[first].Get(WorkspaceHeader) != "1" || f.hdrs[second].Get(WorkspaceHeader) != "2" {
		t.Fatalf("workspace headers %q then %q", f.hdrs[first].Get(WorkspaceHeader), f.hdrs[second].Get(WorkspaceHeader))
	}
	if f.hdrs[first].Get("Authorization") != "Bearer tok" || f.hdrs[first].Get("X-Quests-Source") != "mcp" {
		t.Fatalf("auth headers: %v", f.hdrs[first])
	}
	if !strings.Contains(f.seen[first], "status=active%2Cexpired") || !strings.Contains(f.seen[first], "limit=20") || !strings.Contains(f.seen[first], "summary=1") {
		t.Fatalf("defaults not applied: %s", f.seen[first])
	}
	if !strings.Contains(f.seen[second], "limit=100") || !strings.Contains(f.seen[second], "status=completed") {
		t.Fatalf("limit must be capped at 100: %s", f.seen[second])
	}
}

func TestErrorsAreToolResults(t *testing.T) {
	f := &fakeAPI{fail: map[string]int{"GET /api/quests/9": 404, "PATCH /api/quests/9": 403}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "get_quest", `{"id":9}`),
		call(2, "update_quest", `{"id":9,"title":"t"}`),
		call(3, "update_quest", `{"id":9}`),
		call(4, "get_quest", `{"id":"x"}`),
		call(5, "update_quest", `{"id":1,"status":"bogus"}`),
	)
	for i, want := range []string{"404: nope", "403: nope", "422: provide at least one field", "400: bad arguments", "422: bad status"} {
		text, isErr := result(t, resps[i])
		if !isErr || !strings.HasPrefix(text, want) {
			t.Errorf("call %d: %q (isError=%v) want prefix %q", i+1, text, isErr, want)
		}
	}
	if text, _ := result(t, resps[0]); !strings.Contains(text, "list_workspaces") {
		t.Errorf("404 should hint at workspaces: %s", text)
	}
}

func TestGetQuestCutsDescriptionAndSummarizesSteps(t *testing.T) {
	long := strings.Repeat("я", 9000)
	quest := map[string]any{"id": 5, "title": "q", "status": "active", "description": long, "progress_label": "1 / 2",
		"steps": []any{
			map[string]any{"id": 11, "title": "a", "description": "big body", "done": true, "progress_current": 1, "progress_total": 1},
			map[string]any{"id": 12, "title": "b", "description": "other body", "done": false, "progress_current": 0, "progress_total": 1},
		}}
	b, _ := json.Marshal(quest)
	f := &fakeAPI{routes: map[string]string{"GET /api/quests/5": string(b)}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s, call(1, "get_quest", `{"id":5}`), call(2, "get_quest", `{"id":5,"offset":8000,"max_chars":2000,"include":["step_text"]}`))

	text, _ := result(t, resps[0])
	var got map[string]any
	_ = json.Unmarshal([]byte(text), &got)
	if got["description_len"].(float64) != 9000 || got["description_truncated"] != true || got["description_next_offset"].(float64) != 4000 {
		t.Fatalf("cut: %v", got)
	}
	if len([]rune(got["description"].(string))) != 4000 {
		t.Fatalf("description length %d", len([]rune(got["description"].(string))))
	}
	steps := got["steps"].([]any)
	if len(steps) != 2 || steps[0].(map[string]any)["description"] != nil || steps[0].(map[string]any)["description_len"].(float64) != 8 {
		t.Fatalf("steps must be summaries: %v", steps)
	}

	text, _ = result(t, resps[1])
	got = nil
	_ = json.Unmarshal([]byte(text), &got)
	if got["description_truncated"] != false || len([]rune(got["description"].(string))) != 1000 {
		t.Fatalf("tail: %v", got["description_truncated"])
	}
	if got["steps"].([]any)[1].(map[string]any)["description"] != "other body" {
		t.Fatalf("step_text: %v", got["steps"])
	}
}

func TestUpdateStepsReportsEachOp(t *testing.T) {
	f := &fakeAPI{
		routes: map[string]string{
			"POST /api/quests/4/steps":        `{"id":4,"progress_label":"0 / 2","steps":[{"id":40},{"id":41}]}`,
			"PATCH /api/quests/4/steps/40":    `{"id":4,"progress_label":"1 / 2","steps":[]}`,
			"DELETE /api/quests/4/steps/40":   `{"id":4,"progress_label":"0 / 1","steps":[]}`,
			"PATCH /api/quests/4/steps/41/zz": `{}`,
		},
		fail: map[string]int{"PATCH /api/quests/4/steps/99": 404},
	}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s, call(1, "update_steps", `{"quest_id":4,"ops":[
		{"op":"add","title":"new"},
		{"op":"update","step_id":40,"progress_current":1},
		{"op":"update","step_id":99,"title":"x"},
		{"op":"update","title":"no id"},
		{"op":"delete","step_id":40},
		{"op":"bogus"}
	]}`))
	text, isErr := result(t, resps[0])
	if isErr {
		t.Fatal(text)
	}
	var got struct {
		OK      bool             `json:"ok"`
		Label   string           `json:"progress_label"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	wantOK := []bool{true, true, false, false, true, false}
	if len(got.Results) != 6 {
		t.Fatalf("results %v", got.Results)
	}
	for i, w := range wantOK {
		if got.Results[i]["ok"] != w {
			t.Errorf("op %d ok=%v want %v: %v", i, got.Results[i]["ok"], w, got.Results[i])
		}
	}
	if got.Results[0]["step_id"].(float64) != 41 || got.Label != "0 / 1" {
		t.Fatalf("add result / final label: %v %q", got.Results[0], got.Label)
	}
	if !strings.HasPrefix(got.Results[2]["error"].(string), "404:") {
		t.Fatalf("failed op keeps the API error: %v", got.Results[2])
	}
}

func TestCreateNoteRefusedWithoutCategory(t *testing.T) {
	notes := `[{"id":1,"title":"Гайды","is_category":true,"description":"# Как что-то настроить","parent_id":null},
		{"id":2,"title":"Деплой","is_category":true,"description":"","parent_id":1},
		{"id":3,"title":"a note","is_category":false,"parent_id":2}]`
	f := &fakeAPI{routes: map[string]string{"GET /api/notes": notes, "POST /api/notes": `{"id":9}`}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "create_note", `{"title":"t"}`),
		call(2, "create_note", `{"title":"t","parent_id":1}`),
		call(3, "create_note", `{"title":"t","no_category":true}`),
		call(4, "list_note_categories", `{}`),
	)
	text, isErr := result(t, resps[0])
	if !isErr || !strings.Contains(text, "note=1 Гайды") || !strings.Contains(text, "note=2 Гайды / Деплой") || !strings.Contains(text, "Как что-то настроить") {
		t.Fatalf("refusal: %s", text)
	}
	for i := 1; i <= 2; i++ {
		if text, isErr := result(t, resps[i]); isErr || !strings.Contains(text, `"id":9`) {
			t.Fatalf("create %d: %s", i, text)
		}
	}
	if text, _ := result(t, resps[3]); !strings.Contains(text, `"children":1`) {
		t.Fatalf("categories: %s", text)
	}
}

func TestSetNoteFlagRouting(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{"PUT /api/notes/5/outdated": `{}`, "PATCH /api/notes/5": `{}`}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "set_note_flag", `{"id":5,"flag":"outdated","reason":"old"}`),
		call(2, "set_note_flag", `{"id":5,"flag":"private","value":false}`),
		call(3, "set_note_flag", `{"id":5,"flag":"weird"}`),
	)
	if text, isErr := result(t, resps[0]); isErr || !strings.Contains(text, `"value":true`) {
		t.Fatal(text)
	}
	if _, isErr := result(t, resps[1]); isErr {
		t.Fatal("private flag")
	}
	if text, isErr := result(t, resps[2]); !isErr || !strings.Contains(text, "unknown flag") {
		t.Fatal(text)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.seen, " | ")
	if !strings.Contains(joined, "PUT /api/notes/5/outdated") || !strings.Contains(joined, "PATCH /api/notes/5?") {
		t.Fatalf("routing: %s", joined)
	}
}

func TestDryRunEmitPool(t *testing.T) {
	s, _ := newTestServer(t, &fakeAPI{})
	resps := rpc(t, s,
		call(1, "dry_run_emit_pool", `{"command":"echo '{\"title\":\"T\",\"steps\":[{\"title\":\"a\"}],\"junk\":1}'"}`),
		call(2, "dry_run_emit_pool", `{"command":"exit 3"}`),
		call(3, "dry_run_emit_pool", `{"command":"echo hi"}`),
		call(4, "dry_run_emit_pool", `{"command":"true"}`),
		call(5, "dry_run_emit_pool", `{}`),
	)
	parse := func(i int) map[string]any {
		text, _ := result(t, resps[i])
		var m map[string]any
		_ = json.Unmarshal([]byte(text), &m)
		return m
	}
	if m := parse(0); m["ok"] != true || m["steps_count"].(float64) != 1 || m["ignored_fields"].([]any)[0] != "junk" {
		t.Fatalf("good: %v", m)
	}
	if m := parse(1); m["ok"] != false || !strings.Contains(m["error"].(string), "non-zero exit (3)") {
		t.Fatalf("exit: %v", m)
	}
	if m := parse(2); m["ok"] != false || !strings.Contains(m["error"].(string), "invalid JSON") {
		t.Fatalf("not json: %v", m)
	}
	if m := parse(3); m["ok"] != true || m["quest"] != nil {
		t.Fatalf("nothing printed: %v", m)
	}
	if text, isErr := result(t, resps[4]); !isErr || !strings.Contains(text, "exactly one of") {
		t.Fatal(text)
	}
}

func TestUploadAndAttachmentOwner(t *testing.T) {
	s, _ := newTestServer(t, &fakeAPI{})
	resps := rpc(t, s,
		call(1, "upload_attachment", `{"file_path":"/x"}`),
		call(2, "upload_attachment", `{"file_path":"/x","quest":1,"note":2}`),
		call(3, "upload_attachment", `{"file_path":"/definitely/missing","quest":1}`),
		call(4, "icon", `{"action":"set","quest":1}`),
	)
	for i, want := range []string{"422: provide exactly one of", "422: provide exactly one of", "422: cannot read", "400: bad arguments"} {
		if text, isErr := result(t, resps[i]); !isErr || !strings.HasPrefix(text, want) {
			t.Errorf("call %d: %q want prefix %q", i+1, text, want)
		}
	}
}

func TestTextCutByRunes(t *testing.T) {
	got, total, trunc := text("абвгде", 2, 1)
	if got != "бв" || total != 6 || !trunc {
		t.Fatalf("%q %d %v", got, total, trunc)
	}
	got, _, trunc = text("абв", 10, 99)
	if got != "" || trunc {
		t.Fatalf("offset past the end: %q %v", got, trunc)
	}
}

func TestOldServerGetsAHint(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{"GET /api/quests": `[{"id":1}]`, "GET /api/notes": `[]`}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s, call(1, "list_quests", `{}`), call(2, "list_notes", `{}`))
	for i := range resps {
		if text, isErr := result(t, resps[i]); !isErr || !strings.HasPrefix(text, "501: the Quests server is older") {
			t.Errorf("call %d: %q", i+1, text)
		}
	}
}

func TestCreateQuestNeedsAnExplicitQuestline(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"POST /api/quests":    `{"id":9,"status":"active","progress_label":"0 / 0"}`,
		"GET /api/questlines": `[{"id":3,"title":"Quests"}]`,
		"GET /api/categories": `[{"id":5,"slug":"fun","label":"Развлечения"}]`,
	}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "create_quest", `{"title":"a"}`),
		call(2, "create_quest", `{"title":"b","questline":"  "}`),
		call(3, "create_quest", `{"title":"c","questline":"none"}`),
		call(4, "create_quest", `{"title":"d","questline":"Quests","tag_ids":[1]}`),
		call(5, "create_quest", `{"title":"e","questline":"none","category":"fun","tag_ids":[1]}`),
	)
	for i := 0; i < 2; i++ {
		if text, isErr := result(t, resps[i]); !isErr || !strings.Contains(text, "questline is required") {
			t.Fatalf("call %d should be refused: %s", i+1, text)
		}
	}
	text, isErr := result(t, resps[2])
	if isErr || !strings.Contains(text, "unsorted") || !strings.Contains(text, "no tags") {
		t.Fatalf("standalone, no category, no tags → both hints: %s", text)
	}
	if text, isErr := result(t, resps[3]); isErr || strings.Contains(text, `"hint"`) {
		t.Fatalf("questline + tags → no hint: %s", text)
	}
	if text, isErr := result(t, resps[4]); isErr || strings.Contains(text, `"hint"`) {
		t.Fatalf("category + tags → no hint: %s", text)
	}
}

func TestInstructionsTellToLayOutWork(t *testing.T) {
	got := Instructions()
	for _, want := range []string{"list_questlines", "list_tags", `"none"`, "0003-script-graphs", "automation.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("instructions miss %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "guide") {
		t.Fatalf("no pointer to a vault note: a note may not exist, the instructions stand on their own:\n%s", got)
	}
}

func TestScriptToolListsCreatesAndBinds(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"GET /api/scripts":           `[{"id":3,"title":"счёт","version":1,"subjects":["step"],"needs_runner":true}]`,
		"GET /api/scripts/3":         `{"id":3,"title":"счёт","version":1,"graph":{"nodes":[],"edges":[]}}`,
		"GET /api/script-samples":    `[{"id":"step-once","title":"once","subject":"step","graph":{"nodes":[{"id":1}],"edges":[]}}]`,
		"POST /api/scripts":          `{"id":9,"version":1,"subjects":["step"],"needs_runner":true}`,
		"POST /api/scripts/validate": `{"ok":true,"problems":[],"needs_runner":true,"subjects":["step"]}`,
		"PUT /api/steps/11/script":   ``,
		"DELETE /api/notes/7/script": ``,
	}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "script", `{"action":"list"}`),
		call(2, "script", `{"action":"get","id":3}`),
		call(3, "script", `{"action":"create","title":"из образца","sample":"step-once"}`),
		call(4, "script", `{"action":"validate","graph":{"nodes":[],"edges":[]}}`),
		call(5, "script", `{"action":"bind","subject":"step","id":11,"script_id":9}`),
		call(6, "script", `{"action":"unbind","subject":"note","id":7}`),
		call(7, "script", `{"action":"create","title":"x"}`),
	)
	if text, isErr := result(t, resps[0]); isErr || !strings.Contains(text, `"id":3`) || !strings.Contains(text, `"total":1`) {
		t.Fatalf("list: %s", text)
	}
	if text, isErr := result(t, resps[1]); isErr || !strings.Contains(text, `"graph"`) {
		t.Fatalf("get: %s", text)
	}
	if text, isErr := result(t, resps[2]); isErr || !strings.Contains(text, `"id":9`) {
		t.Fatalf("create from sample: %s", text)
	}
	if text, isErr := result(t, resps[3]); isErr || !strings.Contains(text, `"ok":true`) {
		t.Fatalf("validate: %s", text)
	}
	if text, isErr := result(t, resps[4]); isErr || !strings.Contains(text, `"bound":true`) {
		t.Fatalf("bind: %s", text)
	}
	if text, isErr := result(t, resps[5]); isErr || !strings.Contains(text, `"unbound":true`) {
		t.Fatalf("unbind: %s", text)
	}
	if text, isErr := result(t, resps[6]); !isErr || !strings.Contains(text, "graph or sample") {
		t.Fatalf("create without graph must be refused: %s", text)
	}
}

func TestHookToolListsCreatesAndDetaches(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"GET /api/hooks":       `[{"id":4,"name":"лог","events":["quest_completed"],"script_id":3,"enabled":true}]`,
		"GET /api/hooks/4":     `{"id":4,"name":"лог","script_id":3}`,
		"GET /api/hook-events": `{"aliases":{"complete":["quest_completed"]},"kinds":["quest_completed"]}`,
		"POST /api/hooks":      `{"id":5,"events":["quest_completed"],"script_id":3}`,
		"PATCH /api/hooks/4":   `{"id":4,"quest_id":null}`,
		"DELETE /api/hooks/4":  ``,
	}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "hook", `{"action":"list"}`),
		call(2, "hook", `{"action":"get","id":4}`),
		call(3, "hook", `{"action":"events"}`),
		call(4, "hook", `{"action":"create","events":["complete"],"script_id":3,"run_on":"dev"}`),
		call(5, "hook", `{"action":"update","id":4,"quest_id":0}`),
		call(6, "hook", `{"action":"delete","id":4}`),
		call(7, "hook", `{"action":"create","events":["complete"]}`),
		call(8, "hook", `{"action":"update","id":4}`),
	)
	if text, isErr := result(t, resps[0]); isErr || !strings.Contains(text, `"total":1`) {
		t.Fatalf("list: %s", text)
	}
	if text, isErr := result(t, resps[2]); isErr || !strings.Contains(text, `"aliases"`) {
		t.Fatalf("hook_events: %s", text)
	}
	if text, isErr := result(t, resps[3]); isErr || !strings.Contains(text, `"id":5`) {
		t.Fatalf("hook_create: %s", text)
	}
	if text, isErr := result(t, resps[4]); isErr || !strings.Contains(text, `"id":4`) {
		t.Fatalf("hook_update: %s", text)
	}
	if text, isErr := result(t, resps[5]); isErr || !strings.Contains(text, `"deleted":true`) {
		t.Fatalf("hook_delete: %s", text)
	}
	if text, isErr := result(t, resps[6]); !isErr || !strings.Contains(text, "script_id") {
		t.Fatalf("hook_create without a script must be refused: %s", text)
	}
	if text, isErr := result(t, resps[7]); !isErr || !strings.Contains(text, "at least one field") {
		t.Fatalf("an empty update must be refused: %s", text)
	}
}

func TestCreateTemplateNeedsAnExplicitQuestline(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{"POST /api/templates": `{"id":4}`}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "create_template", `{"title":"t"}`),
		call(2, "create_template", `{"title":"t","questline":"none"}`),
	)
	if text, isErr := result(t, resps[0]); !isErr || !strings.Contains(text, "questline is required") {
		t.Fatalf("should be refused: %s", text)
	}
	if text, isErr := result(t, resps[1]); isErr || !strings.Contains(text, "unsorted") || !strings.Contains(text, "no tags") {
		t.Fatalf("standalone template → hints: %s", text)
	}
}

func TestTrashToolListsAndRestores(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"GET /api/trash/quests":           `[{"kind":"quest","id":5,"title":"q","deleted_at":"2026-10-17T10:00:00Z"}]`,
		"GET /api/trash/notes":            `[{"kind":"note","id":9,"title":"n","deleted_at":"2026-10-17T11:00:00Z"}]`,
		"POST /api/trash/notes/9/restore": `{"id":9}`,
	}}
	s, _ := newTestServer(t, f)
	resps := rpc(t, s,
		call(1, "trash", `{"action":"list"}`),
		call(2, "trash", `{"action":"restore","kind":"note","id":9}`),
		call(3, "trash", `{"action":"restore","kind":"note"}`),
	)
	text, isErr := result(t, resps[0])
	if isErr || !strings.Contains(text, `"total":2`) || strings.Index(text, `"id":9`) > strings.Index(text, `"id":5`) {
		t.Fatalf("list must hold both kinds, newest first: %s", text)
	}
	if text, isErr := result(t, resps[1]); isErr || !strings.Contains(text, `"restored":true`) {
		t.Fatalf("restore: %s", text)
	}
	if text, isErr := result(t, resps[2]); !isErr || !strings.Contains(text, "needs kind") {
		t.Fatalf("restore without an id must be refused: %s", text)
	}
}
