package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/store"
)

// A template whose emit is a script (ADR-0003). The emit tick still drives the period (rolls, retries, limits, the quest that
// is made); only the answer comes from a graph run instead of a job: the run is found again by its key, and its sinks leave
// the result on the run (sinkTemplate), which the tick takes once, like a finished job.

// emitScript is the script bound to a template's emit, or nil.
func emitScript(ctx context.Context, st *store.Store, templateID int64) (*execution.Binding, error) {
	return st.Execution.BindingFor(ctx, execution.SubjectTemplate, templateID)
}

// emitResult of a run, as the sinks write it.
type scriptEmitResult struct {
	Create json.RawMessage `json:"create,omitempty"`
	Error  *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// sinkTemplate leaves what a template graph decided on the run: the quest document, or the error of the attempt.
func (r *CheckRunner) sinkTemplate(ctx context.Context, subj runSubject, n graph.Node, in map[string]graph.Token) error {
	if subj.RunID == 0 {
		return errors.New("приёмник шаблона вне запуска")
	}
	var res scriptEmitResult
	if n.Type == "sink.template_create" {
		doc, ok := in["doc"].Value.(json.RawMessage)
		if !ok {
			return errors.New("нет документа квеста")
		}
		res.Create = doc
	} else {
		var p struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(n.Params, &p)
		msg, _ := in["message"].Value.(string)
		res.Error = &struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		}{Kind: p.Kind, Message: msg}
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return r.Store.Execution.SetRunResult(ctx, subj.RunID, string(raw))
}

// execEmitScript runs the template's emit script for a period. Like execEmitRemote it is asynchronous: the first call starts
// the run and answers "not ready", a later one finds it finished and takes the result once (the run is deleted, so a retry
// starts a new one).
func execEmitScript(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string, b execution.Binding) (out emitOutput, info execInfo, err error, ready bool) {
	info.Status = "error"
	key := fmt.Sprintf("emit:%d|%s|%d.%d", tmpl.ID, periodKey, b.ScriptID, b.Version)
	run, found, lerr := st.Execution.RunByKey(ctx, key)
	if lerr != nil {
		return emitOutput{}, info, lerr, true
	}
	if !found {
		r := &CheckRunner{Store: st}
		raw, gerr := st.Execution.ScriptGraph(ctx, b.ScriptID, b.Version)
		if gerr != nil {
			return emitOutput{}, info, fmt.Errorf("скрипт шаблона: %w", gerr), true
		}
		g, gerr := graph.Decode(raw)
		if gerr != nil {
			return emitOutput{}, info, fmt.Errorf("скрипт шаблона: %w", gerr), true
		}
		tid, id := tmpl.ID, b.ScriptID
		if _, serr := r.startRun(ctx, fmt.Sprintf("script:%d:%d", b.ScriptID, b.Version), &id, g, runSubject{Type: "template", ID: tmpl.ID, Period: periodKey}, runBinding{
			Label: strings.ToLower(strings.TrimSpace(tmpl.EmitRunOn.String)), Kind: execution.JobKindEmitPool, Title: "шаблон: " + tmpl.Title,
			Dedupe: "emit:" + key, Timeout: emitPoolTimeout, ValidFor: emitJobValidFor, TemplateID: &tid, Key: key,
		}); serr != nil {
			log.Printf("emit script: start template %d: %v", tmpl.ID, serr)
		}
		return emitOutput{}, info, nil, false
	}
	if run.State == execution.RunWaiting {
		return emitOutput{}, info, nil, false // a device still has the job (or it is late)
	}
	// Finished: take the result, once.
	if derr := st.Execution.DeleteRun(ctx, run.ID); derr != nil {
		log.Printf("emit script: delete run %d: %v", run.ID, derr)
	}
	if run.FinishedAt != nil {
		info.Duration = run.FinishedAt.Sub(run.CreatedAt)
	}
	if run.State == execution.RunFailed {
		return emitOutput{}, info, fmt.Errorf("запуск скрипта не удался: %s", run.Error), true
	}
	if run.Result == "" {
		info.Status = "ok" // the graph ended without making a quest: no quest this period
		return emitOutput{}, info, nil, true
	}
	var res scriptEmitResult
	if jerr := json.Unmarshal([]byte(run.Result), &res); jerr != nil {
		return emitOutput{}, info, jerr, true
	}
	if res.Error != nil {
		if res.Error.Kind == "timeout" || res.Error.Kind == "bad_json" {
			info.Status = res.Error.Kind
		}
		return emitOutput{}, info, errors.New(res.Error.Message), true
	}
	parsed, perr := parseEmitOutput(res.Create)
	if perr != nil {
		info.Status = "bad_json"
		return emitOutput{}, info, perr, true
	}
	info.Status = "ok"
	return parsed, info, nil, true
}

// emitUsesPool says whether a template makes its quest by a command or a script, not from its own fields.
func emitUsesPool(ctx context.Context, st *store.Store, tmpl templateRow) bool {
	if tmpl.EmitPoolCommand.Valid && strings.TrimSpace(tmpl.EmitPoolCommand.String) != "" {
		return true
	}
	b, err := emitScript(ctx, st, tmpl.ID)
	return err == nil && b != nil
}
