package schedule

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// A test run from the script editor (quest=489, «Запустить на…»): the graph runs over the entity the person picked, as it
// would for a binding, and lands in the run log with source "test". Unless it is live, its sinks write nothing (dryHost).

// tryTimeout and tryValidFor bound a test's Code node: how long it may run, and how long it may wait for its runner.
const (
	tryTimeout  = 60 * time.Second
	tryValidFor = 2 * time.Minute
)

// scriptTitle names a script for a job title, or says its number when it cannot be read.
func (r *CheckRunner) scriptTitle(ctx context.Context, id int64) string {
	if sc, err := r.Store.Execution.GetScript(ctx, id); err == nil && sc.Title != "" {
		return "«" + sc.Title + "»"
	}
	return "script=" + strconv.FormatInt(id, 10)
}

// testEvent makes up the payload of an event of this kind about the subject, with the fields `source.event` reads.
func testEvent(kind, subjType string, subjID int64) map[string]any {
	ev := map[string]any{"kind": kind, "title": "проверка из редактора", "detail": "событие придумано для проверки скрипта"}
	// The family is in the name of the kind (the catalog in platform/hooks names them so).
	switch {
	case strings.HasPrefix(kind, "quest_"), kind == "status_changed":
		ev["quest_id"] = subjID
	case strings.HasPrefix(kind, "step_"):
		ev["step_id"] = subjID
	case strings.HasPrefix(kind, "note_"):
		ev["note_id"] = subjID
	case strings.HasPrefix(kind, "values_"):
		ev["owner_kind"], ev["owner_id"] = subjType, subjID
	case strings.HasPrefix(kind, "attachment_"):
		ev["owner_type"], ev["owner_id"] = subjType, subjID
	}
	return ev
}

// TryRun starts a test run and returns its id; the run log has the rest.
func (r *CheckRunner) TryRun(ctx context.Context, in execution.TryStart) (int64, error) {
	g, reg, err := r.expand(ctx, in.Graph)
	if err != nil {
		return 0, err
	}
	sid := in.ScriptID
	ref := "script:" + strconv.FormatInt(in.ScriptID, 10) + ":" + strconv.Itoa(in.Version)
	if len(in.Inline) > 0 {
		ref = execution.GraphRefInline
	}
	b := runBinding{
		Label: in.Label, Kind: execution.JobKindOnce, Title: "Проверка · " + r.scriptTitle(ctx, in.ScriptID),
		Timeout: tryTimeout, ValidFor: tryValidFor,
	}
	id := in.SubjectID
	switch in.SubjectType {
	case "step":
		b.StepID = &id
	case "note":
		b.NoteID = &id
	case "quest":
		b.QuestID = &id
	case "template":
		b.TemplateID = &id
	}
	if g.RunsOnRunnerIn(reg) && in.Label == "" {
		return 0, errors.New("в скрипте есть код: выберите раннер")
	}
	if graphHasArgs(g) {
		return r.tryEmbedded(ctx, g, reg, in, ref, b)
	}
	subj := runSubject{Type: in.SubjectType, ID: in.SubjectID, SourceKind: execution.RunSourceTest, SourceID: in.ScriptID,
		Dry: !in.Live, Inline: in.Inline}
	if subj.Type == "" {
		subj.Type = "workspace"
	}
	if in.EventKind != "" {
		subj.Event = testEvent(in.EventKind, in.SubjectType, in.SubjectID)
	}
	// Every test is its own run: the dedupe key that keeps a check from piling up must not merge two clicks.
	b.Dedupe = "test:" + strconv.FormatInt(in.ScriptID, 10) + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)
	runID, _, err := r.startRunID(ctx, ref, &sid, g, subj, b)
	if err != nil {
		return 0, err
	}
	if runID == 0 {
		return 0, errors.New("запуск не сохранился")
	}
	return runID, nil
}

func graphHasArgs(g graph.Graph) bool {
	for _, n := range g.Nodes {
		if n.Type == "args" {
			return true
		}
	}
	return false
}

// tryEmbedded tests a script made for texts: the arguments go in, Result comes out, a Code node goes to the runner.
func (r *CheckRunner) tryEmbedded(ctx context.Context, g graph.Graph, reg graph.Registry, in execution.TryStart, ref string, b runBinding) (int64, error) {
	run, reqs, out, advErr := graph.BeginEmbedded(ctx, g, reg, in.Args, in.Scope, r.fetcher(&in.ScriptID))
	if run == nil {
		return 0, advErr
	}
	snap, err := run.Snapshot()
	if err != nil {
		return 0, err
	}
	sid := in.ScriptID
	kind := in.SubjectType
	if kind == "" {
		kind = "text"
	}
	id, err := r.Store.Execution.CreateRun(ctx, execution.NewScriptRun{
		GraphRef: ref, ScriptID: &sid, SubjectType: kind, SubjectID: in.SubjectID, Snapshot: snap,
		SourceKind: execution.RunSourceTest, SourceID: in.ScriptID, Dry: !in.Live, GraphInline: in.Inline,
	})
	if err != nil {
		return 0, err
	}
	if out != "" {
		_ = r.Store.Execution.SetRunResult(ctx, id, out)
	}
	if len(reqs) == 0 {
		state, errText := runState(run, advErr)
		return id, r.Store.Execution.SaveRun(ctx, id, state, snap, errText)
	}
	b.Dedupe = "test:" + strconv.FormatInt(id, 10)
	for i, req := range reqs {
		if _, err := r.enqueueCode(ctx, id, req, b, len(reqs) > 1, i); err != nil {
			_ = r.Store.Execution.SaveRun(ctx, id, execution.RunFailed, snap, err.Error())
			return 0, err
		}
	}
	return id, nil
}
