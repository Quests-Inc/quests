package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/platform/pgtest"
	"github.com/valirum/quests/go/internal/store"
)

func TestListEmitAttemptsEndpoint(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	pgtest.Seed(t, s.Store.DB, `INSERT INTO questtemplate (id, title, created_at, updated_at) VALUES (7, 't', now(), now())`)
	for i, status := range []string{"error", "ok"} {
		if err := s.Store.RecordEmitAttempt(t.Context(), store.EmitAttempt{
			TemplateID: 7, PeriodKey: "2026-10-02", At: time.Now(), Attempt: i + 1,
			Status: status, Items: i, PickedRefs: []string{"mail:1"}, Message: "m",
		}); err != nil {
			t.Fatal(err)
		}
	}

	get := func(path string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, out := get("/api/templates/7/emit-attempts?limit=1")
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, out)
	}
	rows := out["attempts"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["status"] != "ok" {
		t.Fatalf("want newest row only, got %v", rows)
	}
	if code, _ := get("/api/templates/999/emit-attempts"); code != http.StatusNotFound {
		t.Fatalf("missing template: %d want 404", code)
	}
}
