package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"testing"
)

// The web client reads these keys by name: the shape of a template on the wire is part of the API.
func TestTemplateJSONShapeAndCopy(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	do := func(method, path string, body any) []byte {
		var rd *bytes.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rd = bytes.NewReader(raw)
		} else {
			rd = bytes.NewReader(nil)
		}
		r := httptest.NewRequest(method, path, rd)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code >= 300 {
			t.Fatalf("%s %s = %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	keys := func(raw []byte) []string {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%v: %s", err, raw)
		}
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}

	var created struct {
		ID int64 `json:"id"`
	}
	raw := do("POST", "/api/templates", map[string]any{
		"title": "Шаблон", "enabled": false,
		"steps": []map[string]any{{"title": "один"}, {"title": "два", "progress_max": 3}},
	})
	_ = json.Unmarshal(raw, &created)

	want := []string{"automated", "category_color", "category_id", "category_label", "category_slug", "created_at",
		"deadline_time", "description", "duration_seconds", "emit_chance", "emit_limits", "emit_mode", "emit_pool_command",
		"emit_pool_last_outcome", "emit_run_on", "emit_window_end", "emit_window_start", "enabled", "freq", "id",
		"overlay_hidden", "pinned", "questline_color", "questline_icon", "questline_icon_url", "questline_id",
		"questline_title", "significance", "sort_order", "steps", "tags", "timezone", "title", "updated_at", "weekdays"}
	if got := keys(raw); !slices.Equal(got, want) {
		t.Errorf("template keys = %v\nwant %v", got, want)
	}

	var got struct {
		Steps []map[string]any `json:"steps"`
	}
	_ = json.Unmarshal(do("GET", "/api/templates/"+strconv.FormatInt(created.ID, 10), nil), &got)
	stepKeys := []string{"check_command", "check_interval_seconds", "description", "id", "progress_max", "progress_min",
		"run_mode", "run_on", "sort_order", "template_id", "title", "wait_previous"}
	if len(got.Steps) != 2 {
		t.Fatalf("steps = %d", len(got.Steps))
	}
	sk := make([]string, 0)
	for k := range got.Steps[0] {
		sk = append(sk, k)
	}
	sort.Strings(sk)
	if !slices.Equal(sk, stepKeys) {
		t.Errorf("step keys = %v\nwant %v", sk, stepKeys)
	}

	// a copy keeps the steps, switches itself off and gets a new id
	var cp struct {
		ID      int64            `json:"id"`
		Title   string           `json:"title"`
		Enabled bool             `json:"enabled"`
		Steps   []map[string]any `json:"steps"`
	}
	_ = json.Unmarshal(do("POST", "/api/templates/"+strconv.FormatInt(created.ID, 10)+"/copy", nil), &cp)
	if cp.ID == created.ID || cp.ID == 0 || cp.Title != "Шаблон (копия)" || cp.Enabled || len(cp.Steps) != 2 {
		t.Errorf("copy = %+v", cp)
	}
}
