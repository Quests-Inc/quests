package httpapi

import (
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"testing"
)

func TestDeadlineAndNoopStepsDoNotRepeatCompletionHistory(t *testing.T) {
	h, s := secretsServer(t)
	status, q := secretReq(h, "POST", "/api/quests", map[string]any{"title": "Only deadline changed", "steps": []any{map[string]any{"title": "One", "progress_current": 1, "progress_total": 1}}})
	if status != 201 {
		t.Fatal(status, q)
	}
	qid := jsonNum(q["id"])
	steps := q["steps"].([]any)
	step := steps[0].(map[string]any)
	sid := jsonNum(step["id"])
	ctx := wsctx.With(t.Context(), 1)
	count := func() int {
		var n int
		if err := s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM questchangelog WHERE quest_id=?", qid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	if code, out := secretReq(h, "PATCH", "/api/quests/"+qid, map[string]any{"deadline_at": "2027-01-01T12:00:00Z"}); code != 200 {
		t.Fatal(code, out)
	}
	afterDeadline := count()
	if afterDeadline-before != 1 {
		t.Fatalf("deadline changes: got %d", afterDeadline-before)
	}
	path := fmt.Sprintf("/api/quests/%s/steps/%s", qid, sid)
	if code, out := secretReq(h, "PATCH", path, map[string]any{"title": "One", "progress_current": 1, "progress_total": 1}); code != 200 {
		t.Fatal(code, out)
	}
	if count() != afterDeadline {
		t.Fatal("noop created history")
	}
	if code, out := secretReq(h, "PATCH", path, map[string]any{"title": "Renamed completed step"}); code != 200 {
		t.Fatal(code, out)
	}
	var kind string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT kind FROM questchangelog WHERE quest_id=? ORDER BY id DESC LIMIT 1", qid).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind == "step_completed" {
		t.Fatal("renaming a completed step reported completion")
	}
}
