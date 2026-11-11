package journal

import (
	"errors"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func TestResolveEventDeadline(t *testing.T) {
	start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
	got, err := ResolveEventDeadline(start, end, EventEdgeStart, EventDirBefore, 3*86400)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("before start: got %s", got)
	}
	got, err = ResolveEventDeadline(start, end, EventEdgeEnd, EventDirAfter, 2*3600)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2027, 1, 2, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("after end: got %s", got)
	}
	if _, err := ResolveEventDeadline(start, end, "window", EventDirBefore, 0); err == nil {
		t.Fatal("expected a bad edge")
	}
}

func TestEventStoreRoundTrip(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	start := time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	created, err := st.CreateEvent(ctx, Event{
		Title: "  День  ", Description: "заметка", StartsAt: start, EndsAt: end, Color: "nope", Icon: "snowflake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "День" || created.Color != "#5a8a9a" || created.Icon != "calendar" {
		t.Fatalf("normalized: %+v", created)
	}
	if EventToRead(created).Title != "День" {
		t.Fatal("read title")
	}
	list, err := st.ListEvents(ctx)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list: %v %v", list, err)
	}
	if _, err := st.GetEvent(ctx, created.ID+99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if _, err := st.CreateEvent(ctx, Event{Title: "  ", StartsAt: start, EndsAt: end}); err == nil {
		t.Fatal("empty title")
	}
	if _, err := st.CreateEvent(ctx, Event{Title: "x", StartsAt: end, EndsAt: start}); err == nil {
		t.Fatal("end before start")
	}

	now := timeutil.NowUTC()
	q, err := st.CreateQuest(ctx, Quest{
		Title: "к сроку", Status: StatusActive, Significance: SigCommon, CreatedAt: now, UpdatedAt: now,
		Steps: []Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyEvent(ctx, &q, &created.ID, "", "", 3600); err != nil {
		t.Fatal(err)
	}
	if q.EventTitle == nil || *q.EventTitle != "День" || q.DeadlineAt == nil {
		t.Fatalf("phrase: %+v", q)
	}
	q, err = st.UpdateQuest(ctx, q, "updated", "срок")
	if err != nil {
		t.Fatal(err)
	}
	created.Title = "День 2"
	created.EndsAt = end.Add(time.Hour)
	created.Color = "#c47a20"
	created.Icon = "flag"
	if _, err := st.UpdateEvent(ctx, created); err != nil {
		t.Fatal(err)
	}
	q, err = st.GetQuest(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q.EventTitle == nil || *q.EventTitle != "День 2" {
		t.Fatalf("recomputed title: %+v", q.EventTitle)
	}

	if err := st.DeleteEvent(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	q, err = st.GetQuest(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q.EventID != nil {
		t.Fatal("phrase should drop with the event")
	}
	if q.DeadlineAt == nil {
		t.Fatal("resolved deadline stays")
	}
	if err := st.DeleteEvent(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := st.UpdateEvent(ctx, created); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	plain, err := st.CreateQuest(ctx, Quest{
		Title: "без срока", Description: "текст", Status: StatusActive, Significance: SigCommon,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	allDay, err := st.QuestToEvent(ctx, plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !allDay.AllDay || allDay.Title != "без срока" {
		t.Fatalf("all day: %+v", allDay)
	}
	if _, err := st.GetQuest(ctx, plain.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("trashed quest still visible: %v", err)
	}

	at := start
	timed, err := st.CreateQuest(ctx, Quest{
		Title: "со сроком", Status: StatusActive, Significance: SigCommon,
		CreatedAt: now, UpdatedAt: now, DeadlineAt: &at,
	})
	if err != nil {
		t.Fatal(err)
	}
	point, err := st.QuestToEvent(ctx, timed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if point.AllDay || !point.StartsAt.Equal(start) || !point.EndsAt.Equal(start) {
		t.Fatalf("point: %+v", point)
	}
	if _, err := st.QuestToEvent(ctx, timed.ID+99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing quest: %v", err)
	}
	var cleared Quest
	if err := st.ApplyEvent(ctx, &cleared, nil, "", "", 0); err != nil || cleared.EventID != nil {
		t.Fatalf("clear: %v %+v", err, cleared)
	}
}
