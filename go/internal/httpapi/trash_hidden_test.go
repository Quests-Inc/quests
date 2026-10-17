package httpapi

import (
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// A quest in the trash is gone from the map, from link labels and from the job queue; restoring brings it all back.
func TestTrashedQuestIsHiddenEverywhereAndComesBack(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := wsctx.With(t.Context(), 1)
	qid := seedQuest(t, s)
	quest, err := s.Store.Journal.GetQuest(ctx, qid)
	if err != nil || len(quest.Steps) != 1 {
		t.Fatalf("quest: %+v %v", quest, err)
	}
	stepID := quest.Steps[0].ID
	note, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "ref", Description: "см. quest=" + strconv.FormatInt(qid, 10)})
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := s.Store.Execution.EnqueueJob(ctx, execution.JobSpec{
		RunnerLabel: "r", Kind: "step", QuestID: &qid, StepID: &stepID, Title: "j", Command: "true",
	})
	if err != nil {
		t.Fatal(err)
	}

	graphHas := func() (quest, step bool) {
		g, err := s.Store.BuildGraph(ctx, "note", note.ID, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range g.Nodes {
			quest = quest || n.Key == "quest:"+strconv.FormatInt(qid, 10)
			step = step || n.Key == "step:"+strconv.FormatInt(stepID, 10)
		}
		return
	}
	stepInCorpus := func() bool {
		docs, err := s.Store.Knowledge.ListLinkCorpus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			if d.Kind == "step" && d.ID == stepID {
				return true
			}
		}
		return false
	}
	jobState := func() string {
		jobs, err := s.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{StepID: stepID})
		if err != nil || len(jobs) != 1 {
			t.Fatalf("jobs: %+v %v", jobs, err)
		}
		return jobs[0].State
	}

	if q, st := graphHas(); !q || !st || !stepInCorpus() {
		t.Fatalf("a live quest must be on the map (quest %v, step %v) and in the corpus", q, st)
	}

	if err := s.Store.Journal.TrashQuest(ctx, qid, "q"); err != nil {
		t.Fatal(err)
	}
	if q, st := graphHas(); q || st {
		t.Fatalf("a trashed quest must leave the map (quest %v, step %v)", q, st)
	}
	if stepInCorpus() {
		t.Fatal("the steps of a trashed quest must not be link labels")
	}
	if _, err := s.Store.Execution.CancelOrphanJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if got := jobState(); got != "canceled" {
		t.Fatalf("the queued job of a trashed quest must be cancelled, state %q (job %d)", got, job.ID)
	}

	if _, err := s.Store.Journal.RestoreQuest(ctx, qid); err != nil {
		t.Fatal(err)
	}
	if q, st := graphHas(); !q || !st || !stepInCorpus() {
		t.Fatalf("a restored quest must be back on the map (quest %v, step %v) and in the corpus", q, st)
	}
}

func TestNoteInTheTrashIsNotOnTheMap(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := wsctx.With(t.Context(), 1)
	qid := seedQuest(t, s)
	note, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "ref", Description: "см. quest=" + strconv.FormatInt(qid, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Knowledge.TrashNote(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	g, err := s.Store.BuildGraph(ctx, "quest", qid, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range g.Nodes {
		if strings.HasPrefix(n.Key, "note:") {
			t.Fatalf("a trashed note must not be on the map: %+v", n)
		}
	}
}
