package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// POST /api/scripts/{id}/eval: a script made of server nodes only computes a value from a text (ADR-0003 §6b).
func TestScriptEvalOverHTTP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	dice, _ := json.Marshal(graph.DiceScript())
	code, out := secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "Кости", "purpose": "embed", "graph": json.RawMessage(dice)})
	if code != http.StatusCreated || out["purpose"] != "embed" {
		t.Fatalf("create the dice script: %d %v", code, out)
	}
	eval := "/api/scripts/" + jsonNum(out["id"]) + "/eval"

	// The short form `_` goes to the first argument of Args; the same value comes back as text.
	for i := 0; i < 30; i++ {
		code, out = secretReq(h, http.MethodPost, eval, map[string]any{"args": map[string]string{"_": "5+3к8"}})
		n, err := strconv.Atoi(jsonStr(out["value"]))
		if code != http.StatusOK || err != nil || n < 8 || n > 29 {
			t.Fatalf("5+3к8: %d %v", code, out)
		}
	}
	// The named form is the same call.
	if code, out = secretReq(h, http.MethodPost, eval, map[string]any{"args": map[string]string{"expr": "2*(1к1+4)"}}); code != http.StatusOK || out["value"] != "10" {
		t.Fatalf("named argument: %d %v", code, out)
	}

	// A bad text is a 422 that says why, not a 500.
	for name, args := range map[string]map[string]string{
		"division by zero": {"_": "1/0"},
		"too many repeats": {"_": "2000к6"},
		"syntax":           {"_": "5+"},
		"no argument":      {},
	} {
		code, out = secretReq(h, http.MethodPost, eval, map[string]any{"args": args})
		if code != http.StatusUnprocessableEntity || out["detail"] == nil {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if code, _ = secretReq(h, http.MethodPost, eval, map[string]any{"args": map[string]string{"_": strings.Repeat("1", 5000)}}); code != http.StatusRequestEntityTooLarge {
		t.Errorf("arguments over 4 KB: %d", code)
	}

	// No such script is a 404; a script with a Code node is refused, it needs a runner.
	if code, _ = secretReq(h, http.MethodPost, "/api/scripts/999999/eval", map[string]any{"args": map[string]string{"_": "1"}}); code != http.StatusNotFound {
		t.Errorf("missing script: %d", code)
	}
	code, out = secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "С кодом", "graph": json.RawMessage(scriptBody)})
	if code != http.StatusCreated {
		t.Fatalf("create a script with code: %d %v", code, out)
	}
	if code, out = secretReq(h, http.MethodPost, "/api/scripts/"+jsonNum(out["id"])+"/eval", map[string]any{"args": map[string]string{"_": "1"}}); code != http.StatusUnprocessableEntity {
		t.Errorf("a script with code must be refused: %d %v", code, out)
	}
}

// A script that is one baked custom node (the starter «Текст: кости») evaluates the same: the node is expanded first.
func TestScriptEvalThroughACustomNode(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	if code, out := secretReq(h, http.MethodPost, "/api/script-custom-nodes/starters", map[string]any{}); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("install the starters: %d %v", code, out)
	}
	code, nodes := secretReqList(t, h, "/api/script-custom-nodes")
	if code != http.StatusOK {
		t.Fatalf("list custom nodes: %d", code)
	}
	var typ string
	for _, n := range nodes {
		if n["title"] == "Текст: кости" {
			typ = "custom." + jsonNum(n["id"])
		}
	}
	if typ == "" {
		t.Fatalf("the dice starter is missing: %v", nodes)
	}
	code, out := secretReq(h, http.MethodPost, "/api/scripts", map[string]any{"title": "Кости из ноды", "graph": map[string]any{"nodes": []map[string]any{{"id": 1, "type": typ}}, "edges": []any{}}})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	code, out = secretReq(h, http.MethodPost, "/api/scripts/"+jsonNum(out["id"])+"/eval", map[string]any{"args": map[string]string{"_": "1к1+1"}})
	if code != http.StatusOK || out["value"] != "2" {
		t.Fatalf("eval through a custom node: %d %v", code, out)
	}
}

func jsonStr(v any) string {
	s, _ := v.(string)
	return s
}
