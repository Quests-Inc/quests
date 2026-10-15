package journal

import (
	"strings"
	"testing"
	"time"
)

func summaryFixture() []Quest {
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	mk := func(id int64, st QuestStatus, age int, steps ...Step) Quest {
		return Quest{
			ID: id, Title: "q", Status: st, Description: strings.Repeat("я", 10),
			UpdatedAt: base.Add(-time.Duration(age) * time.Minute), CreatedAt: base, Steps: steps,
		}
	}
	return []Quest{
		mk(1, StatusActive, 5, Step{ID: 10, Title: "a", ProgressCurrent: 1, ProgressTotal: 1}, Step{ID: 11, Title: "b", ProgressTotal: 1}),
		mk(2, StatusCompleted, 1),
		mk(3, StatusActive, 3),
		mk(4, StatusActive, 3), // same update time as 3: id breaks the tie
		mk(5, StatusExpired, 9),
	}
}

func ids(p QuestSummaryPage) []int64 {
	var out []int64
	for _, it := range p.Items {
		out = append(out, it.ID)
	}
	return out
}

func TestSummarizeFiltersAndOrders(t *testing.T) {
	p, err := SummarizeQuests(summaryFixture(), SummaryQuery{Statuses: []QuestStatus{StatusActive, StatusExpired}})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(p)
	want := []int64{4, 3, 1, 5}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if p.Total != 4 || p.Truncated || p.Next != "" {
		t.Fatalf("page meta %+v", p)
	}
}

func TestSummarizeFieldsOmitBodies(t *testing.T) {
	p, _ := SummarizeQuests(summaryFixture(), SummaryQuery{Statuses: []QuestStatus{StatusActive}})
	var q1 QuestSummary
	for _, it := range p.Items {
		if it.ID == 1 {
			q1 = it
		}
	}
	if q1.DescriptionLen != 10 {
		t.Fatalf("description_len counts runes, got %d", q1.DescriptionLen)
	}
	if q1.NextStep == nil || q1.NextStep.ID != 11 || q1.NextStep.Title != "b" {
		t.Fatalf("next_step %+v", q1.NextStep)
	}
	if q1.StepsDone != 1 || q1.StepsTotal != 2 || q1.ProgressLabel != "1 / 2" {
		t.Fatalf("progress %+v", q1)
	}
	for _, it := range p.Items {
		if it.ID == 3 && it.NextStep != nil {
			t.Fatal("quest without steps has no next_step")
		}
	}
}

func TestSummarizePaginationWalksEverythingOnce(t *testing.T) {
	var seen []int64
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		p, err := SummarizeQuests(summaryFixture(), SummaryQuery{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 5 {
			t.Fatalf("total %d", p.Total)
		}
		seen = append(seen, ids(p)...)
		if !p.Truncated {
			if p.Next != "" {
				t.Fatal("last page must not carry a cursor")
			}
			break
		}
		if p.Next == "" {
			t.Fatal("truncated page needs next")
		}
		cursor = p.Next
	}
	want := []int64{2, 4, 3, 1, 5}
	if len(seen) != len(want) {
		t.Fatalf("walk %v want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("walk %v want %v", seen, want)
		}
	}
}

func TestSummarizeLimitBounds(t *testing.T) {
	many := make([]Quest, 150)
	for i := range many {
		many[i] = Quest{ID: int64(i + 1), Status: StatusActive, UpdatedAt: time.Unix(int64(i), 0), CreatedAt: time.Unix(0, 0)}
	}
	p, _ := SummarizeQuests(many, SummaryQuery{})
	if len(p.Items) != SummaryDefaultLimit {
		t.Fatalf("default limit: %d", len(p.Items))
	}
	p, _ = SummarizeQuests(many, SummaryQuery{Limit: 1000})
	if len(p.Items) != SummaryMaxLimit {
		t.Fatalf("max limit: %d", len(p.Items))
	}
}

func TestSummarizeBadCursor(t *testing.T) {
	for _, c := range []string{"!!!", "bm9kb3Q"} { // not base64 / no "updated.id" pair
		if _, err := SummarizeQuests(summaryFixture(), SummaryQuery{Cursor: c}); err == nil {
			t.Fatalf("cursor %q must be rejected", c)
		}
	}
}

func TestParseStatuses(t *testing.T) {
	got, err := ParseStatuses(" active, expired ,")
	if err != nil || len(got) != 2 || got[0] != StatusActive || got[1] != StatusExpired {
		t.Fatalf("got %v err %v", got, err)
	}
	if _, err := ParseStatuses("active,bogus"); err == nil {
		t.Fatal("unknown status must fail")
	}
}
