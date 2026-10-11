// Package automationhttp is the HTTP face of the automation context (ADR-0001): templates, runners and their jobs,
// secrets and emit attempts. What lives in other contexts or in the scheduler comes in as plain functions that the
// root (httpapi) supplies.
package automationhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/valirum/quests/go/internal/automation"
	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/refs"
)

// Handler serves the automation endpoints.
type Handler struct {
	Store *automation.Store
	Hub   *events.Hub // nil = no live events

	// RequireOwner answers 403 and false unless the caller owns the workspace (identity decides who is the owner).
	RequireOwner func(w http.ResponseWriter, r *http.Request, what string) bool
	// ResolveRefs gives every mention in a text the title of what it points at.
	ResolveRefs func(r *http.Request, list []refs.Ref) []refs.Resolved
	// MaterializeDue creates the quests of every template that is due (the scheduler owns the rules).
	MaterializeDue func(ctx context.Context)
	// EmitTemplate creates a quest from a template right now and returns its id; automation.ErrNotFound-like
	// errors are reported through ErrTemplateNotFound.
	EmitTemplate func(ctx context.Context, id int64, now time.Time) (questID int64, err error)
	// ErrTemplateNotFound is what EmitTemplate returns when the template is gone.
	ErrTemplateNotFound error
	// RunnerDone is called when a runner reports a job finished (the root reacts to its result).
	RunnerDone func(ctx context.Context, job automation.RunnerJob)
}

// Register mounts every route of the context.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/templates", h.listTemplates)
	mux.HandleFunc("POST /api/templates", h.createTemplate)
	mux.HandleFunc("GET /api/templates/{id}", h.getTemplate)
	mux.HandleFunc("PATCH /api/templates/{id}", h.patchTemplate)
	mux.HandleFunc("DELETE /api/templates/{id}", h.deleteTemplate)
	mux.HandleFunc("POST /api/templates/{id}/copy", h.copyTemplate)
	mux.HandleFunc("POST /api/templates/{id}/emit", h.emitTemplate)
	mux.HandleFunc("GET /api/templates/{id}/emit-attempts", h.listEmitAttempts)
	mux.HandleFunc("GET /api/templates/{id}/streak", h.getTemplateStreak)
	h.registerSecretRoutes(mux)
	h.registerRunner(mux)
}

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}
