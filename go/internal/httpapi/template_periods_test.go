package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestTemplatePeriodsRoundTrip(t *testing.T) {
	h := newAttachmentServer(t, "", "", 0).Handler()
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s =%d %s", method, path, w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	created := call("POST", "/api/templates", map[string]any{"title": "Счётчики", "enabled": false, "freq": "monthly", "monthdays": "15,31"}, 201)
	path := "/api/templates/" + strconv.Itoa(int(created["id"].(float64)))
	if call("GET", path, nil, 200)["monthdays"] != "15,31" {
		t.Fatal("lost month days")
	}
	updated := call("PATCH", path, map[string]any{"freq": "cycle", "cycle_length": 4, "cycle_start": "2026-11-13", "cycle_days": "1,2"}, 200)
	copied := call("POST", path+"/copy", nil, 201)
	for _, key := range []string{"freq", "cycle_length", "cycle_start", "cycle_days", "monthdays"} {
		if copied[key] != updated[key] {
			t.Errorf("copy lost %s", key)
		}
	}
	call("PATCH", path, map[string]any{"cycle_days": "5"}, 400)
	if call("GET", path, nil, 200)["cycle_days"] != "1,2" {
		t.Fatal("invalid update changed schedule")
	}
	call("POST", "/api/templates", map[string]any{"title": "Bad", "freq": "monthly", "monthdays": "32"}, 400)
}
