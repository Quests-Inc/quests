package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// A partial PATCH (colour only, as set_icon_color sends) must leave the
// questline's category alone. It used to wipe it: the current category came
// back from GetQuestline as *int64 and matched neither float64 nor int64.
func TestPatchQuestlinePartialKeepsCategory(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	res, err := s.Store.DB.Exec(
		`INSERT INTO questcategory (slug, label, color, created_at) VALUES ('fun', 'fun', '#000000', now())`)
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	catID, _ := res.LastInsertId()
	res, err = s.Store.DB.Exec(
		`INSERT INTO questline (title, category_id, created_at, updated_at) VALUES ('line', ?, now(), now())`, catID)
	if err != nil {
		t.Fatalf("insert questline: %v", err)
	}
	lineID, _ := res.LastInsertId()

	do := func(body map[string]any) map[string]any {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPatch, "/api/questlines/"+strconv.FormatInt(lineID, 10), bytes.NewReader(raw))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("patch = %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	catOf := func(m map[string]any) any { return m["category_id"] }

	got := do(map[string]any{"color": "#c47a20"})
	if got["color"] != "#c47a20" {
		t.Fatalf("color = %v", got["color"])
	}
	if v, ok := catOf(got).(float64); !ok || int64(v) != catID {
		t.Fatalf("category after colour-only patch = %v, want %d", catOf(got), catID)
	}

	// Explicit null still clears it.
	if got := do(map[string]any{"category_id": nil}); catOf(got) != nil {
		t.Fatalf("explicit null: category = %v, want nil", catOf(got))
	}
}
