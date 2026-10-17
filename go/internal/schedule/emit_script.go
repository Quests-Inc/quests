package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/store"
)

// A template's emit runs as a graph (ADR-0003). The tick still owns the period: it starts the run, and on a later tick
// pulls the finished job and resumes the graph. The sinks decode the quest, validate it, record the attempt and the
// retry; the tick then takes that decision once (the run is deleted) and materialize creates the quest. Empty output is
// "nothing this period". A device that never came, and a cancel, close the period with no attempt and no retry: the run
// and the skipped job stay, so nothing is queued again.

// emitScript is the script bound to a template's emit, or nil.
func emitScript(ctx context.Context, st *store.Store, templateID int64) (*execution.Binding, error) {
	return st.Execution.BindingFor(ctx, execution.SubjectTemplate, templateID)
}

// settledEmit is what a template sink leaves on the run for the tick to take. Applied means the attempt and the roll
// are already written; Closed means the period ended without either.
type settledEmit struct {
	Applied bool          `json:"applied,omitempty"`
	Closed  bool          `json:"closed,omitempty"`
	RollID  int64         `json:"roll_id,omitempty"`
	FailMsg string        `json:"fail_msg,omitempty"`
	Spec    *resolvedSpec `json:"spec,omitempty"`
}

// scriptEmitResult is the mailbox an emit sink used to leave, before it recorded the attempt itself. A run that was
// resumed by the previous release and not yet taken is still read.
type scriptEmitResult struct {
	Create json.RawMessage `json:"create,omitempty"`
	Error  *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// sinkTemplate is the real effect of a template graph: create turns the document into a quest spec (or a miss), an
// attempt error records the failure and the retry, and "closed" ends the period without either.
func (r *CheckRunner) sinkTemplate(ctx context.Context, subj runSubject, n graph.Node, in map[string]graph.Token) error {
	if subj.RunID == 0 {
		return errors.New("приёмник шаблона вне запуска")
	}
	var kind string
	if n.Type == "sink.template_error" {
		var p struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(n.Params, &p)
		kind = p.Kind
		if kind == "closed" {
			return r.Store.Execution.SetRunResult(ctx, subj.RunID, `{"closed":true}`)
		}
	}
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	tmpl, err := r.Store.Automation.TemplateRowByID(ctx, ref.ID)
	if err != nil {
		return err
	}
	when := subj.Now
	if when.IsZero() {
		when = timeutil.NowUTC()
	}
	info := execInfo{Status: "error", Duration: subj.Took, Stderr: subj.Stderr}
	var out emitOutput
	var execErr error
	if n.Type == "sink.template_create" {
		doc, _ := in["doc"].Value.(json.RawMessage)
		parsed, perr := parseEmitOutput(doc)
		if perr != nil {
			info.Status = "bad_json"
			execErr = fmt.Errorf("%s\nstdout:\n%s", perr.Error(), truncate(string(doc), 4000))
		} else {
			info.Status = "ok"
			out = parsed
		}
	} else {
		if kind == "timeout" || kind == "bad_json" {
			info.Status = kind
		}
		msg, _ := in["message"].Value.(string)
		execErr = errors.New(msg)
	}
	res, err := settleEmitAttempt(ctx, r.Store, tmpl, subj.Period, when, out, info, execErr)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(settledEmit{Applied: true, RollID: res.RollID, FailMsg: res.FailMsg, Spec: res.Spec})
	if err != nil {
		return err
	}
	return r.Store.Execution.SetRunResult(ctx, subj.RunID, string(raw))
}

// driveScriptEmit runs the script bound to the template.
func driveScriptEmit(ctx context.Context, st *store.Store, tmpl templateRow, period string, now time.Time, b execution.Binding) (emitResult, bool, error) {
	r := &CheckRunner{Store: st}
	raw, err := st.Execution.ScriptGraph(ctx, b.ScriptID, b.Version)
	if err != nil {
		return emitResult{}, false, fmt.Errorf("скрипт шаблона: %w", err)
	}
	g, err := graph.Decode(raw)
	if err != nil {
		return emitResult{}, false, fmt.Errorf("скрипт шаблона: %w", err)
	}
	id := b.ScriptID
	key := fmt.Sprintf("emit:%d|%s|%d.%d", tmpl.ID, period, b.ScriptID, b.Version)
	label := strings.ToLower(strings.TrimSpace(tmpl.EmitRunOn.String))
	return driveEmit(ctx, r, tmpl, period, now, fmt.Sprintf("script:%d:%d", b.ScriptID, b.Version), &id, g, key, "emit:"+key, label)
}

// driveCommandEmit runs the implicit graph a plain emit_pool_command stands for.
func driveCommandEmit(ctx context.Context, st *store.Store, tmpl templateRow, period string, now time.Time) (emitResult, bool, error) {
	r := &CheckRunner{Store: st}
	key := "emit:" + poolJobKey(tmpl.ID, period, tmpl.EmitPoolCommand.String)
	label := strings.ToLower(strings.TrimSpace(tmpl.EmitRunOn.String))
	return driveEmit(ctx, r, tmpl, period, now, implicitPrefix+"template_emit", nil, graph.TemplateEmit(tmpl.EmitPoolCommand.String), key, key, label)
}

// driveEmit starts the run or, once its job has an outcome, resumes it and takes what the sink decided.
func driveEmit(ctx context.Context, r *CheckRunner, tmpl templateRow, period string, now time.Time, ref string, scriptID *int64, g graph.Graph, key, dedupe, label string) (emitResult, bool, error) {
	st := r.Store
	run, found, err := st.Execution.RunByKey(ctx, key)
	if err != nil {
		return emitResult{}, false, err
	}
	if !found {
		tid := tmpl.ID
		if _, serr := r.startRun(ctx, ref, scriptID, g, runSubject{Type: "template", ID: tmpl.ID, Period: period, Now: now}, runBinding{
			Label: label, Kind: execution.JobKindEmitPool, Title: "шаблон: " + tmpl.Title,
			Dedupe: dedupe, Timeout: emitPoolTimeout, ValidFor: emitJobValidFor, TemplateID: &tid, Key: key,
		}); serr != nil {
			return emitResult{}, false, serr
		}
		run, found, err = st.Execution.RunByKey(ctx, key)
		if err != nil {
			return emitResult{}, false, err
		}
		if !found || run.State == execution.RunWaiting {
			return emitResult{}, false, nil
		}
	}
	if run.State == execution.RunWaiting {
		job, jok, jerr := st.Execution.LatestJobByKey(ctx, dedupe)
		if jerr != nil {
			return emitResult{}, false, jerr
		}
		if !jok {
			return emitResult{}, false, nil
		}
		if _, ok := outcomeOf(job); !ok {
			return emitResult{}, false, nil // still out, or the lease lapsed and a result may yet arrive
		}
		r.finishRun(ctx, job, now)
		run, err = st.Execution.GetRun(ctx, run.ID)
		if err != nil {
			return emitResult{}, false, err
		}
	}
	if run.State == execution.RunWaiting {
		return emitResult{}, false, nil
	}
	settled, err := readSettledEmit(ctx, st, tmpl, period, now, run)
	if err != nil {
		return emitResult{}, false, err
	}
	if settled.Closed {
		return emitResult{}, false, nil // the skipped job stays: it is the marker that this period is done
	}
	if derr := st.Execution.DeleteRun(ctx, run.ID); derr != nil {
		log.Printf("emit: delete run %d: %v", run.ID, derr)
	}
	return emitResult{Spec: settled.Spec, RollID: settled.RollID, FailMsg: settled.FailMsg}, true, nil
}

// readSettledEmit is the sink's decision, or — for a run the previous release left in the mailbox — the same decision
// applied now. A run that failed before any sink ran is itself a failed attempt.
func readSettledEmit(ctx context.Context, st *store.Store, tmpl templateRow, period string, now time.Time, run execution.ScriptRun) (settledEmit, error) {
	if run.Result == "" {
		if run.State == execution.RunFailed {
			msg := run.Error
			if msg == "" {
				msg = "запуск скрипта не удался"
			}
			res, err := settleEmitAttempt(ctx, st, tmpl, period, now, emitOutput{}, execInfo{Status: "error"}, errors.New("запуск скрипта не удался: "+msg))
			return settledEmit{Applied: true, RollID: res.RollID, FailMsg: res.FailMsg, Spec: res.Spec}, err
		}
		res, err := settleEmitAttempt(ctx, st, tmpl, period, now, emitOutput{Nothing: true}, execInfo{Status: "ok"}, nil)
		return settledEmit{Applied: true, RollID: res.RollID, FailMsg: res.FailMsg, Spec: res.Spec}, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(run.Result), &probe); err != nil {
		return settledEmit{}, err
	}
	if _, ok := probe["closed"]; ok {
		return settledEmit{Closed: true}, nil
	}
	if _, ok := probe["applied"]; ok {
		var s settledEmit
		err := json.Unmarshal([]byte(run.Result), &s)
		return s, err
	}
	var old scriptEmitResult
	if err := json.Unmarshal([]byte(run.Result), &old); err != nil {
		return settledEmit{}, err
	}
	info := execInfo{Status: "ok"}
	var out emitOutput
	var execErr error
	if old.Error != nil {
		info.Status = "error"
		if old.Error.Kind == "timeout" || old.Error.Kind == "bad_json" {
			info.Status = old.Error.Kind
		}
		execErr = errors.New(old.Error.Message)
	} else {
		parsed, perr := parseEmitOutput(old.Create)
		if perr != nil {
			info.Status = "bad_json"
			execErr = perr
		} else {
			out = parsed
		}
	}
	res, err := settleEmitAttempt(ctx, st, tmpl, period, now, out, info, execErr)
	return settledEmit{Applied: true, RollID: res.RollID, FailMsg: res.FailMsg, Spec: res.Spec}, err
}

// emitUsesPool says whether a template makes its quest by a command or a script, not from its own fields.
func emitUsesPool(ctx context.Context, st *store.Store, tmpl templateRow) bool {
	if tmpl.EmitPoolCommand.Valid && strings.TrimSpace(tmpl.EmitPoolCommand.String) != "" {
		return true
	}
	b, err := emitScript(ctx, st, tmpl.ID)
	return err == nil && b != nil
}
