package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/quests-inc/quests/go/internal/journal"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDeadlineSourceAndPolicyParity(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	var event map[string]any
	w := doJSON(t, h, "POST", "/api/calendar/events", map[string]any{"title": "Релиз", "starts_at": "2030-05-10T10:00:00Z", "ends_at": "2030-05-10T11:00:00Z"}, nil)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	create := func(extra map[string]any) journal.QuestRead {
		t.Helper()
		body := map[string]any{"title": "Подготовить релиз", "duration_seconds": 3600, "expire_on_deadline": false}
		for k, v := range extra {
			body[k] = v
		}
		w := doJSON(t, h, "POST", "/api/quests", body, nil)
		if w.Code != 201 {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var q journal.QuestRead
		if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
			t.Fatal(err)
		}
		return q
	}
	fixed := create(map[string]any{"deadline_at": "2030-05-10T09:00:00Z"})
	relative := create(map[string]any{"event_id": event["id"], "event_edge": "start", "event_dir": "before", "event_offset_seconds": 3600})
	if *fixed.DeadlineAt != *relative.DeadlineAt || *relative.DurationSeconds != 3600 || relative.ExpireOnDeadline {
		t.Fatalf("source changed options: %+v / %+v", fixed, relative)
	}
	patch := func(id int64, body map[string]any) journal.QuestRead {
		t.Helper()
		w := doJSON(t, h, "PATCH", fmt.Sprintf("/api/quests/%d", id), body, nil)
		if w.Code != 200 {
			t.Fatalf("patch: %d %s", w.Code, w.Body.String())
		}
		var q journal.QuestRead
		if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
			t.Fatal(err)
		}
		return q
	}
	relative = patch(relative.ID, map[string]any{"deadline_at": nil, "event_id": event["id"], "duration_seconds": 3600, "expire_on_deadline": false})
	if relative.DurationSeconds == nil || *relative.DurationSeconds != 3600 {
		t.Fatalf("full form save lost event window: %+v", relative)
	}
	relative = patch(relative.ID, map[string]any{"event_offset_seconds": 7200})
	if !strings.Contains(*relative.DeadlineAt, "08:00:00") || *relative.DurationSeconds != 3600 || relative.ExpireOnDeadline {
		t.Fatalf("partial phrase patch: %+v", relative)
	}
	w = doJSON(t, h, "PATCH", fmt.Sprintf("/api/calendar/events/%.0f", event["id"].(float64)), map[string]any{"starts_at": "2030-05-11T10:00:00Z", "ends_at": "2030-05-11T11:00:00Z"}, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	relative = patch(relative.ID, map[string]any{"duration_seconds": 0, "expire_on_deadline": true})
	if !strings.Contains(*relative.DeadlineAt, "2030-05-11T08:00:00") || *relative.DurationSeconds != 0 || !relative.ExpireOnDeadline || relative.TimerTone != nil {
		t.Fatalf("window and policy coupled: %+v", relative)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	hard := create(map[string]any{"deadline_at": past, "duration_seconds": 0, "expire_on_deadline": true})
	soft := create(map[string]any{"deadline_at": past, "duration_seconds": 3600, "expire_on_deadline": false})
	ids, err := s.Store.Journal.ActiveQuestIDsPastDeadline(t.Context(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, hard.ID) || slices.Contains(ids, soft.ID) {
		t.Fatalf("expiry still depends on window: %v", ids)
	}
	if hard.TimerTone == nil || *hard.TimerTone != "overdue" || soft.TimerTone != nil || soft.Urgent {
		t.Fatalf("past deadline presentation: hard=%+v soft=%+v", hard, soft)
	}
	for _, body := range []map[string]any{{"duration_seconds": -1}, {"expire_on_deadline": "yes"}, {"event_id": event["id"], "event_offset_seconds": -1}} {
		w := doJSON(t, h, "PATCH", fmt.Sprintf("/api/quests/%d", fixed.ID), body, nil)
		if w.Code != 422 {
			t.Fatalf("invalid options accepted: %d %s", w.Code, w.Body.String())
		}
	}
}
