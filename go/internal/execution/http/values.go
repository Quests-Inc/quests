package executionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// Named values (a note, a quest, a step, a template, a questline, the workspace). Readable, unlike secrets.
// A workspace value is written by the owner only: it enters every command. Any other value is document data.

type valueJSON struct {
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   any       `json:"value"`
	Formula string    `json:"formula,omitempty"`
	Roll    *rollJSON `json:"roll,omitempty"`
	Error   string    `json:"error,omitempty"`
	From    string    `json:"from,omitempty"`
	ID      int64     `json:"id,omitempty"`
	Title   string    `json:"title,omitempty"`
}

// rollJSON is the embedded script and argument that produced a literal.
type rollJSON struct {
	Script int64  `json:"script"`
	Arg    string `json:"arg"`
}

type valueListJSON struct {
	Own       []valueJSON `json:"own"`
	Inherited []valueJSON `json:"inherited"`
	Effective []valueJSON `json:"effective"`
	Truncated bool        `json:"truncated"`
}

func (h *Handler) registerValueRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspace/values", h.listValues(workspaceSegment))
	mux.HandleFunc("PUT /api/workspace/values/{name}", h.putValue(workspaceSegment))
	mux.HandleFunc("DELETE /api/workspace/values/{name}", h.deleteValue(workspaceSegment))
	mux.HandleFunc("POST /api/workspace/values/{name}/roll", h.rollValue(workspaceSegment))
	for segment := range secretOwnerKinds {
		mux.HandleFunc("GET /api/"+segment+"/{id}/values", h.listValues(segment))
		mux.HandleFunc("PUT /api/"+segment+"/{id}/values/{name}", h.putValue(segment))
		mux.HandleFunc("DELETE /api/"+segment+"/{id}/values/{name}", h.deleteValue(segment))
		mux.HandleFunc("POST /api/"+segment+"/{id}/values/{name}/roll", h.rollValue(segment))
	}
}

func fromMCP(r *http.Request) bool {
	return r.Header.Get("X-Quests-Source") == "mcp"
}

func valueWriteErr(w http.ResponseWriter, err error) {
	var ve *execution.ValueError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusUnprocessableEntity, ve.Msg)
	case errors.Is(err, execution.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, execution.ErrPrivateValue):
		writeErr(w, http.StatusForbidden, "приватная заметка")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) listValues(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if fromMCP(r) && o.Kind == execution.SecretOwnerNote {
			priv, err := h.Store.NotePrivate(r.Context(), o.ID)
			if err != nil {
				valueWriteErr(w, err)
				return
			}
			if priv {
				writeErr(w, http.StatusForbidden, "приватная заметка")
				return
			}
		}
		set, err := h.Store.ResolveNamedValues(r.Context(), o, fromMCP(r))
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, valueList(set, o))
	}
}

func (h *Handler) putValue(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if segment == workspaceSegment && !h.RequireOwner(w, r, "set values") {
			return
		}
		name := r.PathValue("name")
		if err := execution.CheckValueName(name); err != nil {
			valueWriteErr(w, err)
			return
		}
		var body struct {
			Type    string          `json:"type"`
			Value   json.RawMessage `json:"value"`
			Formula string          `json:"formula"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		var (
			stored execution.StoredValue
			err    error
		)
		if formula := strings.TrimSpace(body.Formula); formula != "" {
			stored, err = h.Store.SetNamedFormula(r.Context(), o, name, body.Type, formula)
		} else {
			var canonical string
			canonical, err = execution.ParseNamedValue(body.Type, body.Value)
			if err == nil {
				stored, err = h.Store.SetNamedValue(r.Context(), o, name, body.Type, canonical)
			}
		}
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		j, err := oneValue(stored, execution.SecretOwner{}, "")
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		h.valuesChanged(r, o)
		writeJSON(w, http.StatusOK, j)
	}
}

// rollValue computes an embedded script on the server and writes the result as the literal of a value that already
// exists. The body may name the script and the argument (they are remembered); without a body the value's own source is
// used again. A formula is never overwritten by a roll. The client never sends the number.
func (h *Handler) rollValue(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if segment == workspaceSegment && !h.RequireOwner(w, r, "set values") {
			return
		}
		name := r.PathValue("name")
		if err := execution.CheckValueName(name); err != nil {
			valueWriteErr(w, err)
			return
		}
		var body struct {
			Script int64  `json:"script"`
			Arg    string `json:"arg"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
		}
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		skip := fromMCP(r)
		set, err := h.Store.ResolveNamedValues(r.Context(), o, skip)
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		var cur *execution.StoredValue
		for i := range set.Effective {
			if set.Effective[i].Name == name {
				cur = &set.Effective[i].StoredValue
				break
			}
		}
		switch {
		case cur == nil:
			writeErr(w, http.StatusUnprocessableEntity, "значения «"+name+"» нет: сначала создайте его")
			return
		case cur.Formula != "":
			writeErr(w, http.StatusUnprocessableEntity, "«"+name+"» — формула: источник её не заменяет")
			return
		}
		script, arg := body.Script, strings.TrimSpace(body.Arg)
		if script == 0 && arg == "" {
			script, arg = cur.RollScript, cur.RollArg
		}
		if script < 1 || arg == "" || len(arg) > 200 {
			writeErr(w, http.StatusUnprocessableEntity, "у значения нет источника броска: укажите скрипт и выражение")
			return
		}
		_, g, reg, ok := h.loadEmbedded(w, r, script)
		if !ok {
			return
		}
		if g.RunsOnRunnerIn(reg) {
			writeErr(w, http.StatusUnprocessableEntity, "источнику нужен раннер: запишите значение нодой «Переменная: записать» в скрипте")
			return
		}
		out, ok := h.runEmbedded(w, r, g, reg, map[string]string{"_": arg}, scopeFrom(r, h, set, skip))
		if !ok {
			return
		}
		stored, err := h.Store.SetRolledValue(r.Context(), o, name, cur.Type, out, script, arg)
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		j, err := oneValue(stored, execution.SecretOwner{}, "")
		if err != nil {
			valueWriteErr(w, err)
			return
		}
		h.valuesChanged(r, o)
		writeJSON(w, http.StatusOK, j)
	}
}

func (h *Handler) deleteValue(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if segment == workspaceSegment && !h.RequireOwner(w, r, "remove values") {
			return
		}
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if err := h.Store.DeleteNamedValue(r.Context(), o, r.PathValue("name")); err != nil {
			valueWriteErr(w, err)
			return
		}
		h.valuesChanged(r, o)
		w.WriteHeader(http.StatusNoContent)
	}
}

func valueList(set execution.ValueSet, o execution.SecretOwner) valueListJSON {
	out := valueListJSON{
		Own: []valueJSON{}, Inherited: []valueJSON{}, Effective: []valueJSON{}, Truncated: set.Truncated,
	}
	for _, v := range set.Own {
		if j, err := oneValue(v, execution.SecretOwner{}, ""); err == nil {
			out.Own = append(out.Own, j)
		}
	}
	for _, p := range set.Effective {
		j, err := oneValue(p.StoredValue, p.From, p.Title)
		if err != nil {
			continue
		}
		if p.From != o {
			out.Inherited = append(out.Inherited, j)
		}
		own := j
		if p.From == o {
			own.From, own.ID, own.Title = "", 0, ""
		}
		out.Effective = append(out.Effective, own)
	}
	return out
}

func oneValue(v execution.StoredValue, from execution.SecretOwner, title string) (valueJSON, error) {
	j := valueJSON{Name: v.Name, Type: v.Type, Formula: v.Formula, Error: v.Error}
	if v.Error == "" {
		n, err := v.JSONValue()
		if err != nil {
			return valueJSON{}, err
		}
		j.Value = n
	}
	if v.RollScript > 0 && v.Formula == "" {
		j.Roll = &rollJSON{Script: v.RollScript, Arg: v.RollArg}
	}
	if from.Kind != "" {
		j.From = from.Kind
		j.ID = from.ID
		j.Title = title
	}
	return j, nil
}

// embedScope is the named values of the text an embedded script was clicked in.
func (h *Handler) embedScope(r *http.Request, kind string, id int64) (*graph.ExprScope, error) {
	if !valueHostKind(kind) || id < 1 {
		return nil, &execution.ValueError{Msg: "хозяин текста: note, quest, step, template или questline"}
	}
	o := execution.SecretOwner{Kind: kind, ID: id}
	ok, err := h.Store.SecretOwnerExists(r.Context(), o)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, execution.ErrNotFound
	}
	skip := fromMCP(r)
	if skip && kind == execution.SecretOwnerNote {
		priv, err := h.Store.NotePrivate(r.Context(), id)
		if err != nil {
			return nil, err
		}
		if priv {
			return nil, execution.ErrPrivateValue
		}
	}
	set, err := h.Store.ResolveNamedValues(r.Context(), o, skip)
	if err != nil {
		return nil, err
	}
	return scopeFrom(r, h, set, skip), nil
}

func valueHostKind(kind string) bool {
	switch kind {
	case execution.SecretOwnerNote, execution.SecretOwnerQuest, execution.SecretOwnerStep,
		execution.SecretOwnerTemplate, execution.SecretOwnerQuestline:
		return true
	default:
		return false
	}
}

func scopeFrom(r *http.Request, h *Handler, set execution.ValueSet, skip bool) *graph.ExprScope {
	sc := &graph.ExprScope{Num: map[string]float64{}, Str: map[string]bool{}}
	for _, p := range set.Effective {
		if p.Error != "" {
			continue
		}
		if n, ok := p.Number(); ok {
			sc.Num[p.Name] = n
		} else {
			sc.Str[p.Name] = true
		}
	}
	cache := map[string]execution.ValueSet{}
	sc.Foreign = func(kind string, id int64, name string) (float64, bool, bool, error) {
		if !valueHostKind(kind) || id < 1 {
			return 0, false, false, nil
		}
		if skip && kind == execution.SecretOwnerNote {
			priv, err := h.Store.NotePrivate(r.Context(), id)
			if err != nil {
				return 0, false, false, err
			}
			if priv {
				return 0, false, false, nil
			}
		}
		key := kind + ":" + strconv.FormatInt(id, 10)
		got, ok := cache[key]
		if !ok {
			resolved, err := h.Store.ResolveNamedValues(r.Context(), execution.SecretOwner{Kind: kind, ID: id}, skip)
			if errors.Is(err, execution.ErrNotFound) {
				return 0, false, false, nil
			}
			if err != nil {
				return 0, false, false, err
			}
			cache[key] = resolved
			got = resolved
		}
		for _, p := range got.Effective {
			if p.Name != name || p.Error != "" {
				continue
			}
			if n, ok := p.Number(); ok {
				return n, false, true, nil
			}
			return 0, true, true, nil
		}
		return 0, false, false, nil
	}
	return sc
}

// jobValues is the inherited literals of the job's subject, upper case, for its environment.
func (h *Handler) jobValues(ctx context.Context, j execution.RunnerJob) map[string]string {
	var owner execution.SecretOwner
	switch {
	case j.StepID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerStep, ID: *j.StepID}
	case j.TemplateID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerTemplate, ID: *j.TemplateID}
	case j.NoteID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: *j.NoteID}
	default:
		return nil
	}
	set, err := h.Store.ResolveNamedValues(ctx, owner, false)
	if err != nil {
		return nil
	}
	return set.Env()
}

func (h *Handler) valuesChanged(r *http.Request, o execution.SecretOwner) {
	if h.ValuesChanged != nil {
		h.ValuesChanged(r.Context(), o.Kind, o.ID)
	}
}
