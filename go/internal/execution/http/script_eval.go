package executionhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// Embedded evaluation (ADR-0003 §6b): a script called from a text (`script=N`) computes a value on the server, at once, with
// no job and no writes. Only trusted nodes run; a graph with a Code node is refused. Open to everyone in the workspace
// (it changes nothing), but bounded: one call has a time cap and only a few run at the same moment.
const (
	evalTimeout = 2 * time.Second
	maxEvalArgs = 4 << 10
)

var evalSlots = make(chan struct{}, 8)

func (h *Handler) registerScriptEval(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/scripts/{id}/eval", h.evalScript)
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
	out, version, ok := h.runEmbedded(w, r, id, in.Args, scope)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": out, "version": version})
}

// runEmbedded computes a script made of server nodes only and answers the error itself, so ok=false means the response is
// written. The caller has bounded the arguments and chosen the values the formulas read.
func (h *Handler) runEmbedded(w http.ResponseWriter, r *http.Request, id int64, args map[string]string, scope *graph.ExprScope) (string, int, bool) {
	select {
	case evalSlots <- struct{}{}:
		defer func() { <-evalSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, "слишком много расчётов сразу, попробуйте ещё раз")
		return "", 0, false
	}
	sc, err := h.Store.GetScript(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return "", 0, false
	}
	g, err := graph.Decode(sc.Graph)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return "", 0, false
	}
	reg, customs, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return "", 0, false
	}
	if g.RunsOnRunnerIn(reg) {
		writeErr(w, http.StatusUnprocessableEntity, "в скрипте есть нода кода: встроить в текст можно только скрипт без неё")
		return "", 0, false
	}
	if len(g.CustomUses()) > 0 {
		if g, err = graph.Expand(g, customs, reg); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return "", 0, false
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), evalTimeout)
	defer cancel()
	out, err := graph.EvaluateScoped(ctx, g, reg, args, scope)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return "", 0, false
	}
	return out, sc.Version, true
}
