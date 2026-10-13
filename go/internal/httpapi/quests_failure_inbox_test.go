package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func TestAckFailureInbox(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	ctx := t.Context()
	now := timeutil.NowUTC()

	failed, err := s.Store.Journal.CreateQuest(ctx, journal.Quest{
		Title: "fail", Status: journal.StatusFailed, Significance: journal.SigCommon,
		CreatedAt: now, UpdatedAt: now,
		Steps: []journal.Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.FailureInbox {
		t.Fatal("create-as-failed must enter inbox")
	}

	path := "/api/quests/" + strconv.FormatInt(failed.ID, 10) + "/ack-failure"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ack status %d body %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "failed" {
		t.Fatalf("status=%v want failed", out["status"])
	}
	if out["failure_inbox"] != false {
		t.Fatalf("failure_inbox=%v want false", out["failure_inbox"])
	}

	// Idempotent.
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, path, nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("re-ack status %d", w2.Code)
	}

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/quests/999999/ack-failure", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing quest: %d want 404", missing.Code)
	}

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/quests/nope/ack-failure", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d want 400", bad.Code)
	}
}
