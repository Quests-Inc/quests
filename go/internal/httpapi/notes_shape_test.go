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

// The web client reads these keys by name: the shape of a note on the wire is part of the API.
func TestNoteJSONShape(t *testing.T) {
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
	if err := json.Unmarshal(do("POST", "/api/notes", map[string]any{"title": "Форма", "description": "см. note=1"}), &created); err != nil {
		t.Fatal(err)
	}

	core := []string{"color", "created_at", "custom_icon", "description", "icon", "icon_url", "id", "is_category",
		"is_private", "is_readme", "parent_id", "pinned", "sort_order", "title", "updated_at"}
	verdict := []string{"has_check", "outdated", "outdated_reason"}
	want := func(extra ...string) []string {
		all := slices.Concat(core, verdict, extra)
		sort.Strings(all)
		return all
	}

	var list []json.RawMessage
	if err := json.Unmarshal(do("GET", "/api/notes", nil), &list); err != nil || len(list) == 0 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if got := keys(list[0]); !slices.Equal(got, want()) {
		t.Errorf("list item keys = %v\nwant %v", got, want())
	}
	if err := json.Unmarshal(do("GET", "/api/notes?sort=popular", nil), &list); err != nil || len(list) == 0 {
		t.Fatalf("popular list: %v", err)
	}
	if got := keys(list[0]); !slices.Equal(got, want("popularity")) {
		t.Errorf("popular list item keys = %v\nwant %v", got, want("popularity"))
	}

	one := do("GET", "/api/notes/"+strconv.FormatInt(created.ID, 10), nil)
	if got, w := keys(one), want("backlinks", "check", "children", "refs"); !slices.Equal(got, w) {
		t.Errorf("single note keys = %v\nwant %v", got, w)
	}
	var full struct {
		Check map[string]any `json:"check"`
	}
	_ = json.Unmarshal(one, &full)
	checkKeys := []string{"command", "error", "error_at", "interval_seconds", "last_run_at", "outdated", "outdated_at", "reason", "run_on"}
	got := make([]string, 0, len(full.Check))
	for k := range full.Check {
		got = append(got, k)
	}
	sort.Strings(got)
	if !slices.Equal(got, checkKeys) {
		t.Errorf("check keys = %v\nwant %v", got, checkKeys)
	}
}
