// Package knowledgehttp is the HTTP face of the knowledge context (ADR-0001): notes, their reading statistics and
// freshness checks, and search. What lives in other contexts (resolving a mention to a quest or a template, the change
// log, the attachments of a deleted note) comes in as plain functions that the root (httpapi) supplies.
package knowledgehttp

import (
	"context"
	"net/http"
	"time"

	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/refs"
)

// ChangeEvent is a line for the change log (which belongs to the journal context).
type ChangeEvent struct {
	Kind       string
	EntityType string
	EntityID   int64
	Title      string
	Detail     string
	Coalesce   time.Duration
}

// Handler serves the knowledge endpoints.
type Handler struct {
	Store   *knowledge.Store
	Hub     *events.Hub // nil = no live events
	DataDir string      // where note icons are kept

	// ResolveRefs gives every mention in a text the title of what it points at (it may be a quest, a template…).
	ResolveRefs func(r *http.Request, list []refs.Ref) []refs.Resolved
	// PurgeAttachments removes the attachments of a deleted owner.
	PurgeAttachments func(ctx context.Context, ownerType string, ownerID int64)
	// LogEvent writes to the change log.
	LogEvent func(ctx context.Context, e ChangeEvent)
}

// Register mounts every route of the context handled here.
func (h *Handler) Register(mux *http.ServeMux) {
	h.RegisterNotes(mux)
	h.RegisterNoteCheck(mux)
}

var (
	writeJSON     = httpx.WriteJSON
	requestSource = httpx.RequestSource
)

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}

// ownerNote is the owner kind attachments use for notes.
const ownerNote = "note"
