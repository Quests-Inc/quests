package schedule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
)

// A run of a script graph (ADR-0003), end to end: start it, hand the Code node to a runner as a job, resume the graph when
// the job comes back. The evaluator is in execution/graph; here are the pieces that need the contexts and the clock.

var builtinNodes = graph.Builtin()

// runSubject is the entity a run is about.
type runSubject struct {
	Type string
	ID   int64
}

// runBinding is how the script is attached to its subject: which runner, how long, and what the job is called.
type runBinding struct {
	Label      string
	Kind       string // the job kind the runner reports (poll / once)
	Title      string
	Dedupe     string // at most one open job per key
	Timeout    time.Duration
	ValidFor   time.Duration
	QuestID    *int64
	StepID     *int64
	TemplateID *int64
	NoteID     *int64
}

// A run's graph is named by a reference: "script:<id>:<version>" (a stored script version) or "implicit:<name>" (the graph a
// plain check_command stands for).
const implicitPrefix = "implicit:"

// startRun evaluates the graph as far as it goes and queues a job for every Code node that is ready. It reports whether it
// queued anything: false means an open job for the same binding already exists (nothing piles up while a device is away).
func (r *CheckRunner) startRun(ctx context.Context, ref string, scriptID *int64, g graph.Graph, subj runSubject, b runBinding) (bool, error) {
	run, err := graph.NewRun(g, builtinNodes)
	if err != nil {
		return false, err
	}
	reqs, advErr := run.Advance(ctx, r.hostFor(subj))
	if advErr != nil {
		log.Printf("script run (%s): %v", ref, advErr)
	}
	snap, err := run.Snapshot()
	if err != nil {
		return false, err
	}
	state, errText := runState(run, advErr)
	id, err := r.Store.Execution.CreateRun(ctx, execution.NewScriptRun{GraphRef: ref, ScriptID: scriptID, SubjectType: subj.Type, SubjectID: subj.ID, Snapshot: snap})
	if err != nil {
		return false, err
	}
	if len(reqs) == 0 {
		return true, r.Store.Execution.SaveRun(ctx, id, state, snap, errText)
	}
	for i, req := range reqs {
		created, err := r.enqueueCode(ctx, id, req, b, len(reqs) > 1, i)
		if err != nil {
			_ = r.Store.Execution.SaveRun(ctx, id, execution.RunFailed, snap, err.Error())
			return false, err
		}
		if !created {
			// The same binding already has an open job: this run is not needed.
			_ = r.Store.Execution.DeleteRun(ctx, id)
			return false, nil
		}
	}
	return true, nil
}

// enqueueCode turns a ready Code node into a job.
func (r *CheckRunner) enqueueCode(ctx context.Context, runID int64, req graph.CodeRequest, b runBinding, several bool, i int) (bool, error) {
	dedupe := b.Dedupe
	if several && dedupe != "" {
		dedupe += "#" + strconv.Itoa(req.Node)
	}
	node := req.Node
	_, created, err := r.Store.Execution.EnqueueJob(ctx, execution.JobSpec{
		RunnerLabel: b.Label, Kind: b.Kind, QuestID: b.QuestID, StepID: b.StepID, TemplateID: b.TemplateID, NoteID: b.NoteID,
		Title: b.Title, DedupeKey: dedupe, Command: bakeCommand(req.Text, req.Language),
		TimeoutSeconds: int(b.Timeout / time.Second), ValidFor: b.ValidFor,
		RunID: &runID, RunNode: &node, Env: req.Env,
	})
	return created, err
}

// bakeCommand is the text the runner gets: the code as written, with an interpreter line in front when the node names a
// language and the text has none of its own.
func bakeCommand(text, language string) string {
	if strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), "#!") {
		return text
	}
	switch language {
	case "bash":
		return "#!/usr/bin/env bash\n" + text
	case "python":
		return "#!/usr/bin/env python3\n" + text
	case "node":
		return "#!/usr/bin/env node\n" + text
	}
	return text // sh: the runner's `sh -c`
}

func runState(run *graph.Run, err error) (string, string) {
	switch {
	case err != nil:
		return execution.RunFailed, err.Error()
	case run.Finished():
		return execution.RunDone, ""
	}
	return execution.RunWaiting, ""
}

// outcomeOf maps how a job ended to the output of the Code node it leaves by: a clean exit by `stdout`, anything else by
// `failed` with a document that says which kind of ending it was. ok is false for the states that say nothing (a lost lease may
// still deliver a result, so the run keeps waiting).
func outcomeOf(j execution.RunnerJob) (graph.CodeOutcome, bool) {
	switch j.State {
	case execution.JobSuccess:
		return graph.CodeOutcome{Port: "stdout", Text: j.Stdout}, true
	case execution.JobFail:
		if j.TimedOut {
			return graph.Failure{Kind: graph.FailTimeout, Message: "таймаут"}.Outcome(), true
		}
		exit := -1
		if j.ExitCode != nil {
			exit = *j.ExitCode
		}
		blob := trimCheckOut(j.Stderr, j.Stdout)
		msg := "код " + strconv.Itoa(exit)
		if blob != "" {
			msg += ": " + blob
		}
		return graph.Failure{Kind: graph.FailExit, Message: msg, ExitCode: &exit, Stderr: blob}.Outcome(), true
	case execution.JobTimeout:
		return graph.Failure{Kind: graph.FailTimeout, Message: "таймаут"}.Outcome(), true
	case execution.JobSkipped:
		return graph.Failure{Kind: graph.FailUnreachable, Message: "устройство «" + j.RunnerLabel + "» не вышло на связь"}.Outcome(), true
	case execution.JobAbandoned:
		return graph.Failure{Kind: graph.FailAbandoned, Message: "устройство «" + j.RunnerLabel + "» пропало во время выполнения, результат неизвестен"}.Outcome(), true
	case execution.JobCanceled:
		return graph.Failure{Kind: graph.FailCanceled, Message: "запуск отменён"}.Outcome(), true
	}
	return graph.CodeOutcome{}, false
}

// resumeRun delivers a finished job to its run and goes on with the graph.
func (r *CheckRunner) resumeRun(ctx context.Context, j execution.RunnerJob) {
	if j.RunID == nil || j.RunNode == nil {
		return
	}
	outcome, ok := outcomeOf(j)
	if !ok {
		return
	}
	run, err := r.Store.Execution.GetRun(ctx, *j.RunID)
	if err != nil {
		if !errors.Is(err, execution.ErrNotFound) {
			log.Printf("script run %d: %v", *j.RunID, err)
		}
		return
	}
	if run.State != execution.RunWaiting {
		return
	}
	g, err := r.graphOf(ctx, run)
	if err != nil {
		_ = r.Store.Execution.SaveRun(ctx, run.ID, execution.RunFailed, run.Snapshot, err.Error())
		log.Printf("script run %d: %v", run.ID, err)
		return
	}
	rn, err := graph.Restore(g, builtinNodes, run.Snapshot)
	if err != nil {
		_ = r.Store.Execution.SaveRun(ctx, run.ID, execution.RunFailed, run.Snapshot, err.Error())
		log.Printf("script run %d: %v", run.ID, err)
		return
	}
	subj := runSubject{Type: run.SubjectType, ID: run.SubjectID}
	reqs, advErr := rn.Resume(ctx, r.hostFor(subj), *j.RunNode, outcome)
	r.afterResume(ctx, subj, outcome, g, rn)
	if advErr != nil {
		log.Printf("script run %d: %v", run.ID, advErr)
	}
	snap, err := rn.Snapshot()
	if err != nil {
		log.Printf("script run %d: %v", run.ID, err)
		return
	}
	state, errText := runState(rn, advErr)
	if err := r.Store.Execution.SaveRun(ctx, run.ID, state, snap, errText); err != nil {
		log.Printf("script run %d: %v", run.ID, err)
	}
	// A branch that waited for this result may have made another Code node ready: it gets its job, bound like the first.
	for i, req := range reqs {
		if _, err := r.enqueueCode(ctx, run.ID, req, bindingOfJob(j), true, i); err != nil {
			log.Printf("script run %d: queue node %d: %v", run.ID, req.Node, err)
		}
	}
}

// bindingOfJob rebuilds the binding from a finished job of the same run.
func bindingOfJob(j execution.RunnerJob) runBinding {
	return runBinding{
		Label: j.RunnerLabel, Kind: j.Kind, Title: j.Title, Dedupe: j.DedupeKey,
		Timeout: time.Duration(j.TimeoutSeconds) * time.Second, ValidFor: 24 * time.Hour,
		QuestID: j.QuestID, StepID: j.StepID, TemplateID: j.TemplateID, NoteID: j.NoteID,
	}
}

// graphOf finds the graph a run executes.
func (r *CheckRunner) graphOf(ctx context.Context, run execution.ScriptRun) (graph.Graph, error) {
	if name, ok := strings.CutPrefix(run.GraphRef, implicitPrefix); ok {
		g, ok := graph.Implicit(name, "-") // the text of the Code node matters only when the job is queued
		if !ok {
			return graph.Graph{}, fmt.Errorf("неизвестный неявный граф %q", name)
		}
		return g, nil
	}
	var id int64
	var version int
	if _, err := fmt.Sscanf(run.GraphRef, "script:%d:%d", &id, &version); err != nil {
		return graph.Graph{}, fmt.Errorf("непонятная ссылка на граф %q", run.GraphRef)
	}
	raw, err := r.Store.Execution.ScriptGraph(ctx, id, version)
	if errors.Is(err, execution.ErrNotFound) {
		return graph.Graph{}, errors.New("скрипт удалён, пока задание ждало раннера")
	}
	if err != nil {
		return graph.Graph{}, err
	}
	return graph.Decode(raw)
}

// scriptKind: the jobs of script runs. It is asked before the older kinds, which keep serving the jobs queued before runs
// existed.
type scriptKind struct{ r *CheckRunner }

func (scriptKind) Name() string                    { return "script" }
func (scriptKind) Owns(j execution.RunnerJob) bool { return j.RunID != nil }
func (k scriptKind) Finished(ctx context.Context, j execution.RunnerJob) {
	k.r.resumeRun(ctx, j)
}

// runnerLabel is the runner a binding names, or "" for a script that needs none.
func runnerLabel(on *string) string {
	if on == nil {
		return ""
	}
	return *on
}

// loadScriptGraph reads the graph of the version a binding points at.
func (r *CheckRunner) loadScriptGraph(ctx context.Context, b execution.Binding) (graph.Graph, string, error) {
	raw, err := r.Store.Execution.ScriptGraph(ctx, b.ScriptID, b.Version)
	if err != nil {
		return graph.Graph{}, "", err
	}
	g, err := graph.Decode(raw)
	return g, fmt.Sprintf("script:%d:%d", b.ScriptID, b.Version), err
}

// dispatchScript runs the script bound to a step. Whether it polls or runs once is the step's mode (the schedule); what it
// does with the answer is the script's graph.
func (r *CheckRunner) dispatchScript(ctx context.Context, q journal.Quest, st journal.Step, once bool, b execution.Binding) {
	g, ref, err := r.loadScriptGraph(ctx, b)
	if err != nil {
		log.Printf("checks: script %d for step %d: %v", b.ScriptID, st.ID, err)
		return
	}
	kind, timeout, valid := execution.JobKindPoll, pollTimeout, pollJobValidFor
	if once {
		kind, timeout, valid = execution.JobKindOnce, onceSafetyTimeout, onceJobValidFor
	}
	qid, sid := q.ID, st.ID
	id := b.ScriptID
	_, err = r.startRun(ctx, ref, &id, g, runSubject{Type: "step", ID: st.ID}, runBinding{
		Label: runnerLabel(st.RunOn), Kind: kind, Title: q.Title + " · " + st.Title,
		Dedupe:  "step:" + strconv.FormatInt(st.ID, 10) + ":" + kind,
		Timeout: timeout, ValidFor: valid, QuestID: &qid, StepID: &sid,
	})
	if err != nil {
		log.Printf("checks: start script %d for step %d: %v", b.ScriptID, st.ID, err)
	}
}

// dispatchNoteScript runs the script bound to a note.
func (r *CheckRunner) dispatchNoteScript(ctx context.Context, d knowledge.DueNote, b execution.Binding) {
	g, ref, err := r.loadScriptGraph(ctx, b)
	if err != nil {
		log.Printf("checks: script %d for note %d: %v", b.ScriptID, d.ID, err)
		return
	}
	nid, id := d.ID, b.ScriptID
	_, err = r.startRun(ctx, ref, &id, g, runSubject{Type: "note", ID: d.ID}, runBinding{
		Label: runnerLabel(d.RunOn), Kind: execution.JobKindPoll, Title: "Заметка · " + d.Title,
		Dedupe:  "note:" + strconv.FormatInt(d.ID, 10) + ":poll",
		Timeout: pollTimeout, ValidFor: noteJobValidFor, NoteID: &nid,
	})
	if err != nil {
		log.Printf("checks: start script %d for note %d: %v", b.ScriptID, d.ID, err)
	}
}
