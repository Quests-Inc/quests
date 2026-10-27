package executionhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// The run log (quest=489): every run of a script, bound or embedded, from a hook or a test, with what each node did. Members
// read it like they read scripts; a test run is owner-only, because it runs code.
func (h *Handler) registerRuns(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/runs", h.listRuns)
	mux.HandleFunc("GET /api/runs/{id}", h.getRun)
	mux.HandleFunc("GET /api/run-stats", h.runStats)
	mux.HandleFunc("POST /api/scripts/{id}/try", h.tryScript)
}

// runStats is the latest run of every source of a kind (step, note, template, hook), by its id: the marks on steps and notes.
func (h *Handler) runStats(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	switch kind {
	case execution.RunSourceStep, execution.RunSourceNote, execution.RunSourceTemplate, execution.RunSourceHook:
	default:
		writeErr(w, http.StatusBadRequest, "kind: step, note, template или hook")
		return
	}
	stats, err := h.Store.RunStatsOf(r.Context(), kind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make(map[string]execution.SourceRunStats, len(stats))
	for id, st := range stats {
		out[strconv.FormatInt(id, 10)] = st
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	num := func(k string) int64 { v, _ := strconv.ParseInt(q.Get(k), 10, 64); return v }
	f := execution.RunFilter{
		ScriptID: num("script_id"), SourceKind: q.Get("source_kind"), SourceID: num("source_id"),
		SubjectType: q.Get("subject_type"), SubjectID: num("subject_id"), State: q.Get("state"),
		Before: num("before"), Limit: int(num("limit")),
	}
	list, err := h.Store.ListRuns(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// runNode is a node of the graph a run executed, with what the run did with it.
type runNode struct {
	ID      int                        `json:"id"`
	Type    string                     `json:"type"`
	Params  json.RawMessage            `json:"params,omitempty"`
	State   graph.NodeState            `json:"state,omitempty"`
	Failure string                     `json:"failure,omitempty"`
	Outputs map[string]json.RawMessage `json:"outputs,omitempty"`
	// Exits are the outputs that carried an exit signal (nothing went that way).
	Exits []string `json:"exits,omitempty"`
}

type runDetail struct {
	execution.ScriptRun
	Nodes []runNode             `json:"nodes"`
	Edges []graph.Edge          `json:"edges"`
	Jobs  []execution.RunnerJob `json:"jobs"`
	// GraphError says why the graph could not be read back (a deleted script, a custom node changed since).
	GraphError string `json:"graph_error,omitempty"`
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	run, err := h.Store.GetRun(r.Context(), id)
	if errors.Is(err, execution.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "запуска нет: он мог устареть")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := runDetail{ScriptRun: run, Nodes: []runNode{}, Edges: []graph.Edge{}}
	if g, err := h.graphOfRun(r, run); err != nil {
		out.GraphError = err.Error()
	} else {
		out.Edges = g.Edges
		for _, n := range g.Nodes {
			rn := runNode{ID: n.ID, Type: n.Type, Params: n.Params, State: run.Snapshot.States[n.ID], Failure: run.Snapshot.Failures[n.ID]}
			for _, t := range run.Snapshot.Tokens {
				if t.Node != n.ID {
					continue
				}
				if t.Exit {
					rn.Exits = append(rn.Exits, t.Port)
					continue
				}
				if rn.Outputs == nil {
					rn.Outputs = map[string]json.RawMessage{}
				}
				rn.Outputs[t.Port] = t.Value
			}
			out.Nodes = append(out.Nodes, rn)
		}
	}
	jobs, err := h.Store.ListRunnerJobs(r.Context(), execution.JobFilter{RunID: run.ID, Limit: 20})
	if err == nil {
		out.Jobs = jobs
	} else {
		out.Jobs = []execution.RunnerJob{}
	}
	writeJSON(w, http.StatusOK, out)
}

// graphOfRun reads back the graph a run executed, custom nodes unfolded the way the run saw them.
func (h *Handler) graphOfRun(r *http.Request, run execution.ScriptRun) (graph.Graph, error) {
	var g graph.Graph
	var err error
	switch {
	case run.GraphRef == execution.GraphRefInline:
		g, err = graph.Decode(run.GraphInline)
	case strings.HasPrefix(run.GraphRef, "implicit:"):
		var ok bool
		if g, ok = graph.Implicit(strings.TrimPrefix(run.GraphRef, "implicit:"), "…"); !ok {
			err = errors.New("неизвестный неявный граф")
		}
	default:
		var sid int64
		var ver int
		parts := strings.Split(run.GraphRef, ":")
		if len(parts) == 3 {
			sid, _ = strconv.ParseInt(parts[1], 10, 64)
			ver, _ = strconv.Atoi(parts[2])
		}
		raw, gerr := h.Store.ScriptGraph(r.Context(), sid, ver)
		if gerr != nil {
			return graph.Graph{}, errors.New("версия скрипта недоступна: скрипт удалён или в корзине")
		}
		g, err = graph.Decode(raw)
	}
	if err != nil || len(g.CustomUses()) == 0 {
		return g, err
	}
	reg, customs, err := h.Store.Registry(r.Context())
	if err != nil {
		return g, err
	}
	return graph.Expand(g, customs, reg)
}

// tryScript starts a test run («Запустить на…»): the saved version, or the graph the editor holds when it is sent along.
func (h *Handler) tryScript(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "test scripts") {
		return
	}
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	var in struct {
		Graph   json.RawMessage `json:"graph"`
		Subject *struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		} `json:"subject"`
		Event string            `json:"event"`
		Args  map[string]string `json:"args"`
		Live  bool              `json:"live"`
		On    string            `json:"on"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	if h.TryRun == nil {
		writeErr(w, http.StatusUnprocessableEntity, "раннеры здесь не подключены")
		return
	}
	sc, err := h.Store.GetScript(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return
	}
	raw := sc.Graph
	start := execution.TryStart{ScriptID: sc.ID, Version: sc.Version, Args: in.Args, Live: in.Live, EventKind: in.Event}
	if len(in.Graph) > 0 && string(in.Graph) != "null" {
		raw = in.Graph
		start.Inline = in.Graph
	}
	g, err := graph.Decode(raw)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	reg, _, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	if ps := g.Validate(reg); len(ps) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"detail": "в графе есть ошибки", "problems": ps})
		return
	}
	if msg := h.networkProblem(r.Context(), raw); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, msg)
		return
	}
	start.Graph = g
	if in.Subject != nil {
		start.SubjectType, start.SubjectID = in.Subject.Type, in.Subject.ID
		if in.Subject.Type != "" && in.Subject.Type != "workspace" {
			if scope, err := h.embedScope(r, in.Subject.Type, in.Subject.ID); err == nil {
				start.Scope = scope
			}
		}
	}
	if g.RunsOnRunnerIn(reg) {
		on, err := h.Store.ResolveEmbedRunner(r.Context(), in.On)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		start.Label = on
	}
	run, err := h.TryRun(r.Context(), start)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
}
