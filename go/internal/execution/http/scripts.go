package executionhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
)

// Scripts (ADR-0003). Everyone in the workspace may read them (the command a step runs is readable too); only an owner
// writes, because a script is user code.
func (h *Handler) registerScripts(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/scripts", h.listScripts)
	mux.HandleFunc("POST /api/scripts", h.createScript)
	mux.HandleFunc("POST /api/scripts/validate", h.validateScript)
	mux.HandleFunc("GET /api/scripts/{id}", h.getScript)
	mux.HandleFunc("PUT /api/scripts/{id}", h.updateScript)
	mux.HandleFunc("DELETE /api/scripts/{id}", h.deleteScript)
	mux.HandleFunc("GET /api/scripts/{id}/versions/{version}", h.getScriptVersion)
	mux.HandleFunc("GET /api/script-nodes", h.listScriptNodes)
	mux.HandleFunc("POST /api/script-nodes/ports", h.scriptNodePorts)
	mux.HandleFunc("GET /api/script-samples", h.listScriptSamples)
	mux.HandleFunc("GET /api/script-custom-nodes", h.listCustomNodes)
	mux.HandleFunc("POST /api/script-custom-nodes", h.createCustomNode)
	mux.HandleFunc("POST /api/script-custom-nodes/starters", h.installStarters)
	mux.HandleFunc("GET /api/script-custom-nodes/{id}", h.getCustomNode)
	mux.HandleFunc("PUT /api/script-custom-nodes/{id}", h.updateCustomNode)
	mux.HandleFunc("DELETE /api/script-custom-nodes/{id}", h.deleteCustomNode)
	h.registerBindings(mux)
}

const maxScriptBody = graph.MaxBytes + 16<<10

// scriptErr maps a store error to a response.
func scriptErr(w http.ResponseWriter, err error) {
	var ge *execution.GraphError
	var inUse *execution.ScriptInUse
	var nodeInUse *execution.CustomNodeInUse
	switch {
	case errors.As(err, &inUse):
		writeJSON(w, http.StatusConflict, map[string]any{"detail": err.Error(), "uses": inUse.Uses})
	case errors.As(err, &ge):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"detail": err.Error(), "problems": ge.Problems})
	case apperr.IsBad(err):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, execution.ErrNotFound):
		writeErr(w, http.StatusNotFound, "скрипт не найден")
	case errors.Is(err, execution.ErrCustomVersion):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.As(err, &nodeInUse):
		writeJSON(w, http.StatusConflict, map[string]any{"detail": err.Error(), "used_by": nodeInUse.By})
	case errors.Is(err, execution.ErrScriptVersion):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func scriptID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (h *Handler) listScripts(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListScripts(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	if err := h.Store.MarkTemplates(r.Context(), list); err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) getScript(w http.ResponseWriter, r *http.Request) {
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	sc, err := h.Store.GetScript(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (h *Handler) getScriptVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		writeErr(w, http.StatusBadRequest, "invalid version")
		return
	}
	g, err := h.Store.ScriptGraph(r.Context(), id, v)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "version": v, "graph": g})
}

func decodeScriptBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScriptBody)).Decode(into); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func (h *Handler) createScript(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "create scripts") {
		return
	}
	var in struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Purpose     string          `json:"purpose"`
		Graph       json.RawMessage `json:"graph"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	sc, err := h.Store.CreateScriptFor(r.Context(), in.Title, in.Description, in.Purpose, in.Graph)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (h *Handler) updateScript(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "change scripts") {
		return
	}
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	var in struct {
		Title       *string         `json:"title"`
		Description *string         `json:"description"`
		Purpose     *string         `json:"purpose"`
		Graph       json.RawMessage `json:"graph"`
		Version     int             `json:"version"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	upd := execution.ScriptUpdate{Title: in.Title, Description: in.Description, Purpose: in.Purpose, ExpectedVersion: in.Version}
	if len(in.Graph) > 0 && string(in.Graph) != "null" {
		upd.Graph = in.Graph
	}
	sc, err := h.Store.UpdateScript(r.Context(), id, upd)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (h *Handler) deleteScript(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "delete scripts") {
		return
	}
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteScript(r.Context(), id); err != nil {
		scriptErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateScript checks a graph without saving it: an editor asks while the user draws.
func (h *Handler) validateScript(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Graph json.RawMessage `json:"graph"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	g, err := graph.Decode(in.Graph)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "problems": []graph.Problem{{Message: err.Error()}}})
		return
	}
	reg, _, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	ps := g.Validate(reg)
	if ps == nil {
		ps = []graph.Problem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": len(ps) == 0, "problems": ps, "needs_runner": g.RunsOnRunnerIn(reg), "subjects": g.Subjects()})
}

func (h *Handler) listScriptNodes(w http.ResponseWriter, r *http.Request) {
	reg, _, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reg.Infos())
}

// scriptNodePorts gives the ports a node of a type would have with the parameters, so an editor can draw them as the user
// changes a Parse schema or a Compare type.
func (h *Handler) scriptNodePorts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if !decodeScriptBody(w, r, &in) {
		return
	}
	reg, _, err := h.Store.Registry(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	inputs, outputs, err := reg.Ports(in.Type, in.Params)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inputs": inputs, "outputs": outputs})
}

// Binding a script to a step, a note or a template's emit. The runner, interval and mode stay on the step or the note (they are edited there as
// ever); choosing a script is what takes the command's place, so the command is cleared in the same request.
func (h *Handler) registerBindings(mux *http.ServeMux) {
	for _, seg := range []struct{ path, kind string }{{"steps", execution.SubjectStep}, {"notes", execution.SubjectNote}, {"templates", execution.SubjectTemplate}} {
		mux.HandleFunc("PUT /api/"+seg.path+"/{id}/script", h.bindScript(seg.kind))
		mux.HandleFunc("DELETE /api/"+seg.path+"/{id}/script", h.unbindScript(seg.kind))
	}
	mux.HandleFunc("GET /api/scripts/{id}/uses", h.scriptUses)
}

func (h *Handler) bindScript(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "attach scripts") {
			return
		}
		id, ok := scriptID(w, r)
		if !ok {
			return
		}
		var in struct {
			ScriptID int64 `json:"script_id"`
		}
		if !decodeScriptBody(w, r, &in) {
			return
		}
		if exists, err := h.SubjectExists(r.Context(), kind, id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		} else if !exists {
			writeErr(w, http.StatusNotFound, kind+" not found")
			return
		}
		if err := h.Store.BindScript(r.Context(), kind, id, in.ScriptID); err != nil {
			scriptErr(w, err)
			return
		}
		if err := h.ClearCommand(r.Context(), kind, id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) unbindScript(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "detach scripts") {
			return
		}
		id, ok := scriptID(w, r)
		if !ok {
			return
		}
		if err := h.Store.UnbindScript(r.Context(), kind, id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) scriptUses(w http.ResponseWriter, r *http.Request) {
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	uses, err := h.Store.ScriptUses(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, uses)
}

// listScriptSamples are the examples to start a script from (copied into the workspace by creating a script from one).
func (h *Handler) listScriptSamples(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, graph.Samples())
}

// Custom nodes (quest=458). Everyone in the workspace reads them; only an owner writes, like the scripts that hold them.
func (h *Handler) listCustomNodes(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListCustomNodes(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) getCustomNode(w http.ResponseWriter, r *http.Request) {
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	n, err := h.Store.GetCustomNode(r.Context(), id)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

type customBody struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Inputs      []graph.CustomPort  `json:"inputs"`
	Outputs     []graph.CustomPort  `json:"outputs"`
	Fields      []graph.CustomField `json:"fields"`
	Graph       graph.Graph         `json:"graph"`
}

func (b customBody) custom() graph.Custom {
	return graph.Custom{Title: b.Title, Description: b.Description, Inputs: b.Inputs, Outputs: b.Outputs, Fields: b.Fields, Graph: b.Graph}
}

func (h *Handler) createCustomNode(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "create custom nodes") {
		return
	}
	var in customBody
	if !decodeScriptBody(w, r, &in) {
		return
	}
	n, err := h.Store.CreateCustomNode(r.Context(), in.custom())
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *Handler) updateCustomNode(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "change custom nodes") {
		return
	}
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	var in customBody
	if !decodeScriptBody(w, r, &in) {
		return
	}
	n, err := h.Store.UpdateCustomNode(r.Context(), id, in.custom(), in.Version)
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) deleteCustomNode(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "delete custom nodes") {
		return
	}
	id, ok := scriptID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteCustomNode(r.Context(), id); err != nil {
		scriptErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// installStarters puts the library's starting nodes (the former samples) into the workspace; the ones it already has stay.
func (h *Handler) installStarters(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "create custom nodes") {
		return
	}
	made, err := h.Store.InstallStarters(r.Context())
	if err != nil {
		scriptErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": made})
}
