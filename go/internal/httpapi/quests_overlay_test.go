package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func questPinState(t *testing.T, h http.Handler, qid int64) (pinned, hidden bool) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/quests/"+strconv.FormatInt(qid, 10), nil))
	var out struct {
		Pinned        bool `json:"pinned"`
		OverlayHidden bool `json:"overlay_hidden"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode quest: %v", err)
	}
	return out.Pinned, out.OverlayHidden
}

func pinnedListIDs(t *testing.T, h http.Handler, v string) map[int64]bool {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/quests?pinned="+v, nil))
	var out []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v (%s)", err, w.Body.String())
	}
	ids := map[int64]bool{}
	for _, q := range out {
		ids[q.ID] = true
	}
	return ids
}

// A quest is in the HUD always (pinned), by default, or never (overlay_hidden);
// the states share one column, so switching one on switches the other off.
func TestQuestOverlayTristate(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	qid := seedQuest(t, s)

	check := func(label string, wantPinned, wantHidden bool) {
		t.Helper()
		if p, hd := questPinState(t, h, qid); p != wantPinned || hd != wantHidden {
			t.Fatalf("%s: pinned=%v hidden=%v, want %v/%v", label, p, hd, wantPinned, wantHidden)
		}
	}
	check("fresh", false, false)

	patch(t, h, qid, map[string]any{"overlay_hidden": true})
	check("hidden", false, true)
	if pinnedListIDs(t, h, "true")[qid] {
		t.Fatal("hidden quest must not be in pinned=true")
	}
	if !pinnedListIDs(t, h, "false")[qid] {
		t.Fatal("hidden quest counts as not pinned")
	}

	patch(t, h, qid, map[string]any{"pinned": true})
	check("pinned replaces hidden", true, false)
	if !pinnedListIDs(t, h, "true")[qid] {
		t.Fatal("pinned quest missing from pinned=true")
	}

	patch(t, h, qid, map[string]any{"overlay_hidden": true})
	check("hidden replaces pinned", false, true)

	patch(t, h, qid, map[string]any{"overlay_hidden": false})
	check("back to default", false, false)
}
