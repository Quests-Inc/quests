package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func TestListQuestsSummary(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	ctx := t.Context()
	now := timeutil.NowUTC()

	long := strings.Repeat("x", 5000)
	for _, st := range []journal.QuestStatus{journal.StatusActive, journal.StatusActive, journal.StatusCompleted} {
		if _, err := s.Store.Journal.CreateQuest(ctx, journal.Quest{
			Title: "q", Description: long, Status: st, Significance: journal.SigCommon,
			CreatedAt: now, UpdatedAt: now,
			Steps: []journal.Step{{Title: "first", Description: long, ProgressTotal: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	get := func(url string) (int, []byte) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w.Code, w.Body.Bytes()
	}

	code, body := get("/api/quests?summary=1&status=active,completed&limit=2")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if len(body) > 2000 {
		t.Fatalf("summary page must not carry bodies, got %d bytes", len(body))
	}
	var page journal.QuestSummaryPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 2 || !page.Truncated || page.Next == "" {
		t.Fatalf("page %+v", page)
	}
	if page.Items[0].NextStep == nil || page.Items[0].NextStep.Title != "first" || page.Items[0].DescriptionLen != 5000 {
		t.Fatalf("item %+v", page.Items[0])
	}

	code, body = get("/api/quests?summary=1&status=active,completed&limit=2&cursor=" + page.Next)
	var rest journal.QuestSummaryPage
	if err := json.Unmarshal(body, &rest); code != http.StatusOK || err != nil {
		t.Fatalf("second page %d %v", code, err)
	}
	if len(rest.Items) != 1 || rest.Truncated {
		t.Fatalf("second page %+v", rest)
	}

	if code, _ := get("/api/quests?summary=1&status=nope"); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status: %d", code)
	}
	if code, _ := get("/api/quests?summary=1&cursor=!!!"); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", code)
	}

	// Without summary the old full-array shape is unchanged.
	code, body = get("/api/quests?status=active")
	var full []map[string]any
	if err := json.Unmarshal(body, &full); code != http.StatusOK || err != nil || len(full) != 2 {
		t.Fatalf("legacy list %d %v len=%d", code, err, len(full))
	}
	if _, ok := full[0]["steps"]; !ok {
		t.Fatal("legacy list keeps steps")
	}
}
