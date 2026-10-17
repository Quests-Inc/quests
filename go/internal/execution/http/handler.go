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
	// RunnerDone is called when a runner reports a job finished (the root reacts to its result).
	RunnerDone func(ctx context.Context, job execution.RunnerJob)
}

// Register mounts every route of the context.
func (h *Handler) Register(mux *http.ServeMux) {
	h.registerSecretRoutes(mux)
	h.registerRunner(mux)
	h.registerScripts(mux)
}

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}
