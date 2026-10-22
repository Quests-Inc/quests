// Package executionhttp is the HTTP face of the execution context (ADR-0002): runners, their protocol and jobs, the
// secrets a command runs with, and the scripts (ADR-0003). What lives in other contexts comes in as plain functions that the root (httpapi) supplies.
package executionhttp

import (
	"context"
	"net/http"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
)

// Handler serves the execution endpoints.
type Handler struct {
	Store *execution.Store

	// RequireOwner answers 403 and false unless the caller owns the workspace (identity decides who is the owner).
	RequireOwner func(w http.ResponseWriter, r *http.Request, what string) bool
	// SubjectExists says whether the step or note a script is bound to exists; ClearCommand removes its own command when a
	// script takes over. Both are other contexts' business and come from the root.
	SubjectExists func(ctx context.Context, kind string, id int64) (bool, error)
	ClearCommand  func(ctx context.Context, kind string, id int64) error
	// RunnerDone is called when a runner reports a job finished (the root reacts to its result).
	RunnerDone func(ctx context.Context, job execution.RunnerJob)
}

// Register mounts every route of the context.
func (h *Handler) Register(mux *http.ServeMux) {
	h.registerSecretRoutes(mux)
	h.registerValueRoutes(mux)
	h.registerRunner(mux)
	h.registerScripts(mux)
	h.registerScriptEval(mux)
	h.registerHooks(mux)
}

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}
