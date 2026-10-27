package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/pgtest"
)

func secretsServer(t *testing.T) (http.Handler, *Server) { return secretsServerWith(t, nil) }

// secretsServerWith is secretsServer with a change to the server before its handler is built.
func secretsServerWith(t *testing.T, tweak func(*Server)) (http.Handler, *Server) {
	t.Helper()
	s := newAttachmentServer(t, "", "", 0)
	if tweak != nil {
		tweak(s)
	}
	pgtest.Seed(t, s.Store.DB, `
		INSERT INTO questline (id, title, created_at, updated_at) VALUES (7, 'line', now(), now());
		INSERT INTO questtemplate (id, title, questline_id, created_at, updated_at) VALUES (5, 'tmpl', 7, now(), now())`)
	pgtest.FixSequences(t, s.Store.DatabaseURL)
	return s.Handler(), s
}

func secretReq(h http.Handler, method, path string, body any) (int, map[string]any) {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestSecretsEndpointsAtEveryLevel(t *testing.T) {
	h, s := secretsServer(t)
	qid := seedQuest(t, s)
	var stepID int64
	if err := s.Store.DB.QueryRow(`SELECT id FROM queststep WHERE quest_id = ?`, qid).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec(`UPDATE quest SET questline_id = 7, template_id = 5 WHERE id = ?`, qid); err != nil {
		t.Fatal(err)
	}

	owners := map[string]string{
		"questlines": "/api/questlines/7",
		"templates":  "/api/templates/5",
		"quests":     "/api/quests/" + strconv.FormatInt(qid, 10),
		"steps":      "/api/steps/" + strconv.FormatInt(stepID, 10),
	}
	for name, base := range owners {
		if code, _ := secretReq(h, http.MethodPut, base+"/secrets/"+"K_"+name, map[string]string{"value": "secret-value-" + name}); code != http.StatusNoContent {
			t.Fatalf("PUT %s: %d", name, code)
		}
	}
	// Values never come back; the step lists its own key plus everything inherited, with sources.
	code, out := secretReq(h, http.MethodGet, owners["steps"]+"/secrets", nil)
	if code != http.StatusOK {
		t.Fatalf("GET steps: %d", code)
	}
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte("secret-value")) {
		t.Fatalf("a value leaked into the listing: %s", raw)
	}
	if keys := out["keys"].([]any); len(keys) != 1 || keys[0] != "K_steps" {
		t.Fatalf("own keys = %v", keys)
	}
	inh := out["inherited"].([]any)
	got := map[string]string{}
	for _, it := range inh {
		m := it.(map[string]any)
		got[m["key"].(string)] = m["from"].(string)
	}
	want := map[string]string{"K_questlines": "questline", "K_templates": "template", "K_quests": "quest"}
	if len(got) != 3 || got["K_questlines"] != want["K_questlines"] || got["K_templates"] != want["K_templates"] || got["K_quests"] != want["K_quests"] {
		t.Fatalf("inherited = %v, want %v", got, want)
	}

	if code, _ := secretReq(h, http.MethodDelete, owners["quests"]+"/secrets/K_quests", nil); code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", code)
	}
	if _, out := secretReq(h, http.MethodGet, owners["quests"]+"/secrets", nil); len(out["keys"].([]any)) != 0 {
		t.Fatalf("key not deleted: %v", out)
	}
}

func TestSecretsValidationAndMissingOwners(t *testing.T) {
	h, _ := secretsServer(t)
	for _, base := range []string{"/api/questlines/999", "/api/templates/999", "/api/quests/999", "/api/steps/999"} {
		if code, _ := secretReq(h, http.MethodPut, base+"/secrets/KEY", map[string]string{"value": "v-value"}); code != http.StatusNotFound {
			t.Errorf("PUT %s on a missing owner: %d, want 404", base, code)
		}
		if code, _ := secretReq(h, http.MethodGet, base+"/secrets", nil); code != http.StatusNotFound {
			t.Errorf("GET %s on a missing owner: %d, want 404", base, code)
		}
	}
	if code, _ := secretReq(h, http.MethodPut, "/api/templates/5/secrets/bad-key", map[string]string{"value": "v"}); code != http.StatusUnprocessableEntity {
		t.Errorf("non-env-var key: %d, want 422", code)
	}
	if code, _ := secretReq(h, http.MethodPut, "/api/templates/5/secrets/KEY", map[string]string{"value": ""}); code != http.StatusUnprocessableEntity {
		t.Errorf("empty value: %d, want 422", code)
	}
}

func TestWorkspaceSecretsOverHTTP(t *testing.T) {
	h, s := secretsServer(t)
	qid := seedQuest(t, s)
	var stepID int64
	if err := s.Store.DB.QueryRow(`SELECT id FROM queststep WHERE quest_id = ?`, qid).Scan(&stepID); err != nil {
		t.Fatal(err)
	}

	if code, _ := secretReq(h, http.MethodPut, "/api/workspace/secrets/API_TOKEN", map[string]string{"value": "ws-token-value"}); code != http.StatusNoContent {
		t.Fatalf("put: %d", code)
	}
	if code, _ := secretReq(h, http.MethodPut, "/api/workspace/secrets/bad-key", map[string]string{"value": "v"}); code != http.StatusUnprocessableEntity {
		t.Errorf("a bad key: %d, want 422", code)
	}
	if code, _ := secretReq(h, http.MethodPut, "/api/workspace/secrets/LD_PRELOAD", map[string]string{"value": "v"}); code != http.StatusUnprocessableEntity {
		t.Errorf("a reserved name: %d, want 422", code)
	}
	code, out := secretReq(h, http.MethodGet, "/api/workspace/secrets", nil)
	if code != http.StatusOK || len(out["keys"].([]any)) != 1 || out["keys"].([]any)[0] != "API_TOKEN" {
		t.Fatalf("list: %d %v", code, out)
	}
	if b, _ := json.Marshal(out); bytes.Contains(b, []byte("ws-token-value")) {
		t.Fatal("a value must never come back")
	}
	// The step inherits it, and says where it comes from.
	_, out = secretReq(h, http.MethodGet, "/api/steps/"+strconv.FormatInt(stepID, 10)+"/secrets", nil)
	inh, _ := out["inherited"].([]any)
	if len(inh) != 1 || inh[0].(map[string]any)["key"] != "API_TOKEN" || inh[0].(map[string]any)["from"] != "workspace" {
		t.Fatalf("inherited: %v", out)
	}
	if code, _ := secretReq(h, http.MethodDelete, "/api/workspace/secrets/API_TOKEN", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if _, out = secretReq(h, http.MethodGet, "/api/workspace/secrets", nil); len(out["keys"].([]any)) != 0 {
		t.Fatalf("not deleted: %v", out)
	}
}

func TestNoteSecretsEnvironmentShapeAndMCPPrivacy(t *testing.T) {
	h, s := secretsServer(t)
	pgtest.Seed(t, s.Store.DB, `INSERT INTO note (id,title,parent_id,is_private,created_at,updated_at) VALUES (101,'private parent',NULL,1,now(),now()),(102,'public child',101,0,now(),now())`)
	for path, value := range map[string]string{"/api/notes/101/secrets/PARENT": "parent-secret-value", "/api/notes/102/secrets/OWN": "child-secret-value"} {
		if code, _ := secretReq(h, http.MethodPut, path, map[string]string{"value": value}); code != 204 {
			t.Fatalf("put: %d", code)
		}
	}
	code, out := secretReq(h, http.MethodGet, "/api/notes/102/secrets", nil)
	if code != 200 || len(out["own"].([]any)) != 1 || len(out["inherited"].([]any)) != 1 || len(out["effective"].([]any)) != 2 {
		t.Fatalf("shape: %d %v", code, out)
	}
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte("secret-value")) {
		t.Fatal("secret value leaked")
	}
	for _, id := range []string{"101", "102"} {
		req := httptest.NewRequest(http.MethodGet, "/api/notes/"+id+"/secrets", nil)
		req.Header.Set("X-Quests-Source", "mcp")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if id == "101" {
			if w.Code != 403 {
				t.Fatalf("private note: %d %s", w.Code, w.Body.String())
			}
			continue
		}
		if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("PARENT")) || bytes.Contains(w.Body.Bytes(), []byte("private parent")) {
			t.Fatalf("private inheritance leak: %d %s", w.Code, w.Body.String())
		}
	}
}
