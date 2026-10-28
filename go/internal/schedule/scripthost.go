package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// scriptHost is what the evaluator needs from the rest of the system for one run: what the source nodes read and what the
// sink nodes do. It is the one place where the node catalog meets the contexts.
type scriptHost struct {
	r    *CheckRunner
	subj runSubject
}

func (r *CheckRunner) hostFor(s runSubject) graph.Host {
	if s.Dry {
		return dryHost{scriptHost{r: r, subj: s}}
	}
	return scriptHost{r: r, subj: s}
}

// dryHost is the host of a test run: sources read as usual, sinks write nothing. What a sink would have written is on the
// links into it, which the run's snapshot keeps; Result still hands its text back, since that is the answer of the test.
type dryHost struct{ scriptHost }

func (h dryHost) Sink(ctx context.Context, n graph.Node, in map[string]graph.Token) error {
	if n.Type == "sink.result" {
		return h.scriptHost.Sink(ctx, n, in)
	}
	return nil
}

func (h scriptHost) Source(ctx context.Context, n graph.Node) (map[string]any, error) {
	kind := strings.TrimPrefix(n.Type, "source.")
	if kind == "event" {
		return h.eventSource(n)
	}
	// The source names its entity; without an id it reads the one the run was triggered for (the script's binding).
	var sp graph.SourceParams
	if len(n.Params) > 0 {
		if err := json.Unmarshal(n.Params, &sp); err != nil {
			return nil, fmt.Errorf("параметры источника: %w", err)
		}
	}
	id := sp.ID
	if id == 0 {
		if kind != h.subj.Type {
			return nil, fmt.Errorf("источник %q без id в запуске для %q: укажите id сущности", kind, h.subj.Type)
		}
		id = h.subj.ID
	}
	ref := graph.Ref{Kind: kind, ID: id}
	secrets := graph.Secrets{Of: ref}
	evMap := h.subj.Event
	if evMap == nil {
		evMap = map[string]any{}
	}
	event := any(evMap)
	switch kind {
	case "note":
		note, err := h.r.Store.Knowledge.GetNote(ctx, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ref": ref, "secrets": secrets, "title": note.Title, "event": event}, nil
	case "template":
		t, err := h.r.Store.Automation.TemplateRowByID(ctx, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ref": ref, "secrets": secrets, "title": t.Title, "period": h.subj.Period, "event": event}, nil
	case "step":
		_, st, ok := h.r.stepByID(ctx, id)
		if !ok {
			return nil, errors.New("шаг не найден")
		}
		return map[string]any{"ref": ref, "secrets": secrets, "title": st.Title, "description": st.Description,
			"progress": st.ProgressCurrent, "total": st.ProgressTotal, "done": st.Done, "event": event}, nil
	case "quest":
		q, err := h.r.Store.Journal.GetQuest(ctx, id)
		if err != nil {
			return nil, err
		}
		// Progress is the steps done of the steps there are (what the quest's own label counts); a quest without steps is 0 of 1,
		// so the numbers can be written onto a step as they are.
		done, total, _ := journal.ProgressLabel(q.Steps)
		return map[string]any{"ref": ref, "secrets": secrets, "title": q.Title, "status": string(q.Status),
			"description": q.Description, "progress": int64(done), "total": int64(max(total, 1)), "event": event}, nil
	case "workspace":
		name, err := h.r.Store.Execution.Identity.WorkspaceName(ctx, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ref": ref, "secrets": secrets, "name": name, "event": event}, nil
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
	case "sink.template_create", "sink.template_error":
		return h.r.sinkTemplate(ctx, h.subj, n, in)
	case "sink.done":
		return nil
	case "sink.values":
		return h.r.sinkValues(ctx, in, h.subj.Origin)
	case "sink.value":
		return h.r.sinkValue(ctx, n, in, h.subj.Origin)
	case "sink.result":
		// An embedded script that waited for a runner: what the link shows is kept on the run until the page asks.
		if h.subj.RunID == 0 {
			return nil
		}
		text, _ := in["value"].Value.(string)
		return h.r.Store.Execution.SetRunResult(ctx, h.subj.RunID, text)
	case "sink.http":
		return h.r.sinkHTTP(ctx, n, in)
	case "sink.socket":
		return h.r.sinkSocket(ctx, n, in)
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

// sinkValue writes what a script computed into a named value of the entity the reference points at.
func (r *CheckRunner) sinkValue(ctx context.Context, n graph.Node, in map[string]graph.Token, origin string) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	var p graph.ValueSinkParams
	if err := json.Unmarshal(n.Params, &p); err != nil {
		return err
	}
	var text string
	switch v := in["value"].Value.(type) {
	case string:
		text = v
	case int64:
		text = strconv.FormatInt(v, 10)
	case float64:
		text = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		text = strconv.FormatBool(v)
	default:
		return fmt.Errorf("значение %q: пусто", p.Name)
	}
	owner := execution.SecretOwner{Kind: ref.Kind, ID: ref.ID}
	if ref.Kind == "workspace" {
		owner = execution.WorkspaceSecretOwner(ctx)
	}
	if err := r.Store.Execution.WriteScriptValue(ctx, owner, p.Name, string(p.Type), text); err != nil {
		return fmt.Errorf("значение %q: %w", p.Name, err)
	}
	if r.Hub != nil {
		r.Hub.ValuesChanged(wsctx.From(ctx), events.ValueChange{
			OwnerKind: owner.Kind, OwnerID: owner.ID, Name: p.Name, Type: string(p.Type), Value: in["value"].Value, Origin: origin,
		})
	}
	return nil
}

// eventSource gives a hook script the event that woke it: the common fields, and the ones of its family.
func (h scriptHost) eventSource(n graph.Node) (map[string]any, error) {
	var p graph.EventSourceParams
	if len(n.Params) > 0 {
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return nil, fmt.Errorf("параметры источника: %w", err)
		}
	}
	ev := h.subj.Event
	if ev == nil {
		ev = map[string]any{}
	}
	str := func(k string) string { s, _ := ev[k].(string); return s }
	num := func(k string) int64 { return eventInt(ev, k) }
	raw, _ := json.Marshal(ev)
	out := map[string]any{"kind": str("kind"), "title": str("title"), "detail": str("detail"), "event": json.RawMessage(raw)}
	ref := func(kind string, id int64) {
		if id > 0 {
			r := graph.Ref{Kind: kind, ID: id}
			out["ref"], out["secrets"] = r, graph.Secrets{Of: r}
		}
	}
	switch p.Family {
	case "quest":
		out["quest_id"] = num("quest_id")
		ref("quest", num("quest_id"))
	case "step":
		out["step_id"], out["quest_id"], out["step_title"] = num("step_id"), num("quest_id"), str("step_title")
		ref("step", num("step_id"))
	case "note":
		out["note_id"] = num("note_id")
		ref("note", num("note_id"))
	case "value":
		owner := p.Owner
		if owner == "" {
			owner = "note"
		}
		out["owner_kind"], out["owner_id"], out["name"], out["type"] = str("owner_kind"), num("owner_id"), str("name"), str("type")
		if v, ok := ev["value"]; ok {
			b, _ := json.Marshal(v)
			out["value"] = json.RawMessage(b)
		}
		if str("owner_kind") == owner {
			ref(owner, num("owner_id"))
		}
	case "attachment":
		out["attachment_id"], out["owner_type"], out["owner_id"] = num("attachment_id"), str("owner_type"), num("owner_id")
	}
	return out, nil
}

func (r *CheckRunner) sinkValues(ctx context.Context, in map[string]graph.Token, origin string) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(in["values"].Value)
	if err != nil {
		return err
	}
	owner := execution.SecretOwner{Kind: ref.Kind, ID: ref.ID}
	if ref.Kind == "workspace" {
		owner = execution.WorkspaceSecretOwner(ctx)
	}
	values, err := r.Store.Execution.WriteScriptValues(ctx, owner, raw)
	if err != nil {
		return err
	}
	if r.Hub != nil {
		for _, v := range values {
			var value any
			encoded := json.RawMessage(v.Value)
			if v.Type == "str" {
				value = v.Value
			} else {
				_ = json.Unmarshal(encoded, &value)
			}
			r.Hub.ValuesChanged(wsctx.From(ctx), events.ValueChange{OwnerKind: owner.Kind, OwnerID: owner.ID, Name: v.Name, Type: v.Type, Value: value, Origin: origin})
		}
	}
	return nil
}
