package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// scriptHost is what the evaluator needs from the rest of the system for one run: what the source nodes read and what the
// sink nodes do. It is the one place where the node catalog meets the contexts.
type scriptHost struct {
	r    *CheckRunner
	subj runSubject
}

func (r *CheckRunner) hostFor(s runSubject) graph.Host { return scriptHost{r: r, subj: s} }

func (h scriptHost) Source(ctx context.Context, n graph.Node) (map[string]any, error) {
	kind := strings.TrimPrefix(n.Type, "source.")
	if kind != h.subj.Type {
		return nil, fmt.Errorf("источник %q в запуске для %q", kind, h.subj.Type)
	}
	ref := graph.Ref{Kind: kind, ID: h.subj.ID}
	switch kind {
	case "note":
		note, err := h.r.Store.Knowledge.GetNote(ctx, h.subj.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ref": ref, "secrets": graph.Secrets{}, "title": note.Title}, nil
	case "step":
		_, st, ok := h.r.stepByID(ctx, h.subj.ID)
		if !ok {
			return nil, errors.New("шаг не найден")
		}
		return map[string]any{"ref": ref, "secrets": graph.Secrets{}, "title": st.Title, "description": st.Description,
			"progress": st.ProgressCurrent, "total": st.ProgressTotal, "done": st.Done}, nil
	}
	return nil, fmt.Errorf("источник %q ещё не подключён", n.Type)
}

func (h scriptHost) Sink(ctx context.Context, n graph.Node, in map[string]graph.Token) error {
	switch n.Type {
	case "sink.note_verdict":
		return h.r.sinkNoteVerdict(ctx, in)
	case "sink.note_error":
		return h.r.sinkNoteError(ctx, in)
	case "sink.step_result":
		return h.r.sinkStepResult(ctx, n, in)
	case "sink.step_progress":
		return h.r.sinkStepProgress(ctx, in)
	}
	return fmt.Errorf("приёмник %q ещё не подключён", n.Type)
}

// afterResume is the bookkeeping of the schedule that belongs to the engine, not to a graph: a polled step records that its
// check ran even when the graph wrote nothing to it (a failed command, an answer that was not one).
func (r *CheckRunner) afterResume(ctx context.Context, subj runSubject, o graph.CodeOutcome, g graph.Graph, rn *graph.Run) {
	if subj.Type != "step" || (o.Port != "stdout" && o.Port != "failed" && o.Port != "timeout") {
		return
	}
	for _, n := range g.Nodes {
		if n.Type == "sink.step_progress" && rn.State(n.ID) == graph.StateRan {
			return // the sink recorded it
		}
		if n.Type == "sink.step_result" {
			return // a one-shot step is never checked again
		}
	}
	q, st, ok := r.stepByID(ctx, subj.ID)
	if !ok {
		return
	}
	now := timeutil.NowUTC()
	st.CheckLastRunAt = &now
	_, _ = r.Store.Journal.UpdateStep(ctx, *st, q, "step_progress", st.Title)
}

// sinkStepProgress writes a reading onto a polled step (what apply did after parsing the answer).
func (r *CheckRunner) sinkStepProgress(ctx context.Context, in map[string]graph.Token) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	q, st, ok := r.stepByID(ctx, ref.ID)
	if !ok {
		return nil // deleted meanwhile
	}
	asInt := func(name string) *int {
		if v, ok := in[name].Value.(int64); ok {
			n := int(v)
			return &n
		}
		return nil
	}
	var desc *string
	if d, ok := in["description"].Value.(string); ok {
		desc = &d
	}
	r.applyProgress(ctx, q, st, asInt("progress"), asInt("total"), desc)
	return nil
}

func refOf(in map[string]graph.Token) (graph.Ref, error) {
	ref, ok := in["ref"].Value.(graph.Ref)
	if !ok {
		return graph.Ref{}, errors.New("нет ссылки на сущность")
	}
	return ref, nil
}

// sinkNoteVerdict turns an `outdated` answer into the verdict of a note (what applyNote did after reading it).
func (r *CheckRunner) sinkNoteVerdict(ctx context.Context, in map[string]graph.Token) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	n, err := r.Store.Knowledge.GetNote(ctx, ref.ID)
	if err != nil {
		return nil // deleted meanwhile
	}
	outdated, _ := in["outdated"].Value.(bool)
	reason, _ := in["reason"].Value.(string)
	res := knowledge.NoteCheckResult{Outdated: outdated, Reason: strings.TrimSpace(reason)}
	changed, err := r.Store.Knowledge.ApplyNoteCheckResult(ctx, n.ID, res, timeutil.NowUTC())
	if err != nil {
		return fmt.Errorf("вердикт заметки %d: %w", n.ID, err)
	}
	if !changed {
		return nil
	}
	detail := "актуальна"
	if res.Outdated {
		detail = "устарела"
		if res.Reason != "" {
			detail += ": " + res.Reason
		}
	}
	r.noteChanged(ctx, n.ID, n.Title, detail)
	return nil
}

// sinkNoteError records a broken check on the note and leaves its verdict alone.
func (r *CheckRunner) sinkNoteError(ctx context.Context, in map[string]graph.Token) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	n, err := r.Store.Knowledge.GetNote(ctx, ref.ID)
	if err != nil {
		return nil
	}
	msg, _ := in["message"].Value.(string)
	r.noteError(ctx, n.ID, n.Title, msg)
	return nil
}

// stepByID finds a step and its quest; ok is false when either is gone.
func (r *CheckRunner) stepByID(ctx context.Context, id int64) (journal.Quest, *journal.Step, bool) {
	qid, err := r.Store.Journal.QuestIDForStep(ctx, id)
	if err != nil {
		return journal.Quest{}, nil, false
	}
	q, err := r.Store.Journal.GetQuest(ctx, qid)
	if err != nil {
		return journal.Quest{}, nil, false
	}
	for i := range q.Steps {
		if q.Steps[i].ID == id {
			return q, &q.Steps[i], true
		}
	}
	return q, nil, false
}

// sinkStepResult is what became of a one-shot step's run: done, failed, not started (no device), or stopped by hand. A step
// that was closed meanwhile is left alone, except that a finished command still closes it.
func (r *CheckRunner) sinkStepResult(ctx context.Context, n graph.Node, in map[string]graph.Token) error {
	var p struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(n.Params, &p); err != nil {
		return err
	}
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	q, st, ok := r.stepByID(ctx, ref.ID)
	if !ok {
		return nil // deleted meanwhile
	}
	why, _ := in["why"].Value.(string)
	switch p.Result {
	case "done":
		now := timeutil.NowUTC()
		st.CheckLastRunAt = &now
		r.onceSucceeded(ctx, q, st)
	case "failed":
		if st.Done {
			return nil
		}
		if why == "" {
			why = "запуск не удался"
		}
		r.onceFailed(ctx, q, st, "автошаг «"+st.Title+"»: "+why)
	case "not_started":
		if st.Done {
			return nil
		}
		idle := journal.RunIdle
		st.RunStatus = &idle
		if why != "" {
			why += " — запущу снова, когда появится"
		}
		if _, err := r.Store.Journal.UpdateStepComment(ctx, *st, q, "quest_updated", "устройство недоступно", "автошаг «"+st.Title+"»: "+why); err != nil {
			return fmt.Errorf("шаг %d: %w", st.ID, err)
		}
	case "stopped":
		if st.Done {
			return nil
		}
		if why == "" {
			why = "запуск отменён"
		}
		fail := journal.RunFail
		st.RunStatus = &fail
		if _, err := r.Store.Journal.UpdateStepComment(ctx, *st, q, "quest_updated", "автошаг остановлен", "автошаг «"+st.Title+"»: "+why); err != nil {
			return fmt.Errorf("шаг %d: %w", st.ID, err)
		}
	default:
		return fmt.Errorf("неизвестный итог %q", p.Result)
	}
	return nil
}
