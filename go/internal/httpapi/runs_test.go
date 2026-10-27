package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// The run log over HTTP: a page of runs with titles, one run with its nodes and the values on their outputs, the stats of a
// kind of source, and a test run handed to the scheduler.
func TestRunLogOverHTTP(t *testing.T) {
	var tried execution.TryStart
	h, s := secretsServerWith(t, func(s *Server) {
		s.TryRun = func(_ context.Context, in execution.TryStart) (int64, error) {
			tried = in
			return 77, nil
		}
	})
	ctx := wsctx.With(t.Context(), 1)
	code, sc := secretReq(h, http.MethodPost, "/api/scripts", json.RawMessage(`{"title":"хук-скрипт","graph":`+hookScriptBody+`}`))
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	sid, _ := strconv.ParseInt(jsonNum(sc["id"]), 10, 64)
	snap := graph.Snapshot{
		States: map[int]graph.NodeState{1: graph.StateRan},
		Tokens: []graph.SnapshotToken{{Node: 1, Port: "kind", Value: json.RawMessage(`"quest_completed"`)}, {Node: 1, Port: "ref", Exit: true}},
	}
	run, err := s.Store.Execution.CreateRun(ctx, execution.NewScriptRun{GraphRef: "script:" + strconv.FormatInt(sid, 10) + ":1", ScriptID: &sid,
		SubjectType: "workspace", SubjectID: 1, Snapshot: snap, SourceKind: execution.RunSourceTest, SourceID: sid, Dry: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Execution.SaveRun(ctx, run, execution.RunDone, snap, ""); err != nil {
		t.Fatal(err)
	}

	get := func(path string, into any) int {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		_ = json.Unmarshal(w.Body.Bytes(), into)
		return w.Code
	}
	var list []execution.RunSummary
	if code := get("/api/runs?script_id="+strconv.FormatInt(sid, 10)+"&source_kind=test", &list); code != http.StatusOK || len(list) != 1 ||
		list[0].ScriptTitle != "хук-скрипт" || list[0].SourceTitle != "хук-скрипт" || !list[0].Dry {
		t.Fatalf("list: %d %+v", code, list)
	}
	var detail struct {
		State string `json:"state"`
		Nodes []struct {
			ID      int                        `json:"id"`
			Type    string                     `json:"type"`
			State   string                     `json:"state"`
			Outputs map[string]json.RawMessage `json:"outputs"`
			Exits   []string                   `json:"exits"`
		} `json:"nodes"`
		Jobs []any `json:"jobs"`
	}
	if code := get("/api/runs/"+strconv.FormatInt(run, 10), &detail); code != http.StatusOK || detail.State != "done" || len(detail.Nodes) == 0 {
		t.Fatalf("detail: %d %+v", code, detail)
	}
	first := detail.Nodes[0]
	if first.ID != 1 || first.State != "ran" || string(first.Outputs["kind"]) != `"quest_completed"` || len(first.Exits) != 1 || detail.Jobs == nil {
		t.Fatalf("node 1: %+v", first)
	}
	if code := get("/api/runs/999999", &detail); code != http.StatusNotFound {
		t.Fatalf("missing run: %d", code)
	}
	var stats map[string]execution.SourceRunStats
	if code := get("/api/run-stats?kind=hook", &stats); code != http.StatusOK {
		t.Fatalf("stats: %d", code)
	}
	if code := get("/api/run-stats?kind=nope", &stats); code != http.StatusBadRequest {
		t.Fatalf("bad kind: %d", code)
	}

	// A test of a script with code needs a runner; a server one goes straight to the scheduler.
	if code, _ := secretReq(h, http.MethodPost, "/api/scripts/"+strconv.FormatInt(sid, 10)+"/try", map[string]any{}); code != http.StatusUnprocessableEntity {
		t.Fatalf("code without a runner: %d", code)
	}
	code, srv := secretReq(h, http.MethodPost, "/api/scripts", json.RawMessage(`{"title":"зеркало","graph":{"nodes":[
		{"id":1,"type":"source.event","params":{"family":"quest"}},{"id":2,"type":"sink.value","params":{"name":"m","type":"str"}}],
		"edges":[{"from":{"node":1,"port":"ref"},"to":{"node":2,"port":"ref"}},{"from":{"node":1,"port":"kind"},"to":{"node":2,"port":"value"}}]}}`))
	if code != http.StatusCreated {
		t.Fatalf("server script: %d %v", code, srv)
	}
	sid = int64(srv["id"].(float64))
	code, out := secretReq(h, http.MethodPost, "/api/scripts/"+strconv.FormatInt(sid, 10)+"/try",
		map[string]any{"subject": map[string]any{"type": "quest", "id": 5}, "event": "quest_completed", "live": true})
	if code != http.StatusAccepted || out["run"] != float64(77) || tried.SubjectType != "quest" || tried.EventKind != "quest_completed" || !tried.Live {
		t.Fatalf("try: %d %v %+v", code, out, tried)
	}
	code, _ = secretReq(h, http.MethodPost, "/api/scripts/"+strconv.FormatInt(sid, 10)+"/try", map[string]any{"graph": map[string]any{"nodes": []any{map[string]any{"id": 1, "type": "nope"}}}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("a broken inline graph: %d", code)
	}
}
