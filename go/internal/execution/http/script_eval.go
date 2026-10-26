package executionhttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// Embedded evaluation (ADR-0003 §6b): a script called from a text (`script=N`) computes a value at once, with no job and no
// writes, when it holds only trusted nodes. A script with a Code node (`script=N{…}?on=label`) is started as a run instead:
// the answer is 202 with the run, the code goes to the named runner, and the page asks for the result until it is there.
// Open to everyone in the workspace, but bounded: one call has a time cap and only a few run at the same moment.
const (
	evalTimeout = 2 * time.Second
	maxEvalArgs = 4 << 10
)

var evalSlots = make(chan struct{}, 8)

func (h *Handler) registerScriptEval(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/scripts/{id}/eval", h.evalScript)
	mux.HandleFunc("GET /api/script-runs/{id}", h.embeddedRun)
}

func (h *Handler) evalScript(w http.ResponseWriter, r *http.Request) {
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	var in struct {
		Args map[string]string `json:"args"`
		Host *struct {
			Kind string `json:"kind"`
			ID   int64  `json:"id"`
		} `json:"host"`
		On string `json:"on"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	size := 0
	for k, v := range in.Args {
		size += len(k) + len(v)
	}
	if size > maxEvalArgs {
		writeErr(w, http.StatusRequestEntityTooLarge, "аргументы длиннее 4 КБ")
		return
	}
	var scope *graph.ExprScope
	if in.Host != nil {
		var err error
		scope, err = h.embedScope(r, in.Host.Kind, in.Host.ID)
		if err != nil {
			valueWriteErr(w, err)
			return
		}
	}
	sc, g, reg, ok := h.loadEmbedded(w, r, id)
	if !ok {
		return
	}
	if g.RunsOnRunnerIn(reg) {
		h.startEmbedded(w, r, sc, g, in.Args, in.Host, in.On, scope)
		return
	}
	out, ok := h.runEmbedded(w, r, sc.ID, g, reg, in.Args, scope)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": out, "version": sc.Version})
}

// loadEmbedded reads a script and its graph, custom nodes expanded, and answers the error itself.
func (h *Handler) loadEmbedded(w http.ResponseWriter, r *http.Request, id int64) (execution.Script, graph.Graph, graph.Registry, bool) {
	sc, err := h.Store.GetScript(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return sc, graph.Graph{}, nil, false
	}
	g, err := graph.Decode(sc.Graph)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return sc, g, nil, false
	}
	reg, customs, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return sc, g, nil, false
	}
	if len(g.CustomUses()) > 0 {
		if g, err = graph.Expand(g, customs, reg); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return sc, g, nil, false
		}
	}
	return sc, g, reg, true
}

// runEmbedded computes a graph of trusted nodes on the server. ok=false means the response is written. The caller has bounded
// the arguments and chosen the values the formulas read.
func (h *Handler) runEmbedded(w http.ResponseWriter, r *http.Request, scriptID int64, g graph.Graph, reg graph.Registry, args map[string]string, scope *graph.ExprScope) (string, bool) {
	select {
	case evalSlots <- struct{}{}:
		defer func() { <-evalSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, "слишком много расчётов сразу, попробуйте ещё раз")
		return "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), evalTimeout)
	defer cancel()
	out, err := graph.EvaluateFetch(ctx, g, reg, args, scope, h.Store.NewFetcher(h.Egress, scriptID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return "", false
	}
	return out, true
}

// startEmbedded hands the script with a Code node to a runner and answers 202 with the run to wait for.
func (h *Handler) startEmbedded(w http.ResponseWriter, r *http.Request, sc execution.Script, g graph.Graph, args map[string]string, host *struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}, on string, scope *graph.ExprScope) {
	if h.StartEmbedded == nil {
		writeErr(w, http.StatusUnprocessableEntity, "раннеры здесь не подключены")
		return
	}
	on, err := h.Store.ResolveEmbedRunner(r.Context(), on)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	start := execution.EmbedStart{ScriptID: sc.ID, Version: sc.Version, Graph: g, Args: args, Label: on, Scope: scope}
	if host != nil {
		start.HostKind, start.HostID = host.Kind, host.ID
	}
	run, err := h.StartEmbedded(r.Context(), start)
	switch {
	case errors.Is(err, execution.ErrTooManyEmbedded):
		writeErr(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "state": execution.RunWaiting, "version": sc.Version})
}

// embeddedRun is what a page asks while its link waits for a runner: the state, and the text of Result when it is there.
// Only runs started from a text are readable here.
func (h *Handler) embeddedRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	run, err := h.Store.GetRun(r.Context(), id)
	if err != nil || !strings.HasPrefix(run.Key, execution.EmbedKeyPrefix) {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	out := map[string]any{"state": run.State}
	switch run.State {
	case execution.RunDone:
		out["value"] = run.Result
	case execution.RunFailed:
		out["error"] = run.Error
	}
	writeJSON(w, http.StatusOK, out)
}
