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
	select {
	case evalSlots <- struct{}{}:
		defer func() { <-evalSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, "слишком много расчётов сразу, попробуйте ещё раз")
		return
	}
	sc, err := h.Store.GetScript(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return
	}
	g, err := graph.Decode(sc.Graph)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	reg, customs, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	if g.RunsOnRunnerIn(reg) {
		writeErr(w, http.StatusUnprocessableEntity, "в скрипте есть нода кода: встроить в текст можно только скрипт без неё")
		return
	}
	if len(g.CustomUses()) > 0 {
		if g, err = graph.Expand(g, customs, reg); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), evalTimeout)
	defer cancel()
	out, err := graph.Evaluate(ctx, g, reg, in.Args)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": out, "version": sc.Version})
}
