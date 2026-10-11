// Package journalhttp is the HTTP face of the journal context (ADR-0001): quests and their steps, questlines,
// categories, tags and the change log. What belongs to other contexts (resolving a mention to a note or a template,
// removing the attachments of a deleted quest, knowing that a template exists) comes in as plain functions that the
// root (httpapi) supplies.
package journalhttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/refs"
)

// Handler serves the journal endpoints.
type Handler struct {
	Store   *journal.Store
	Hub     *events.Hub // nil = no live events
	DataDir string      // where questline icons are kept

	// ResolveRefs gives every mention in a text the title of what it points at (a note, a template, a step…).
	ResolveRefs func(r *http.Request, list []refs.Ref) []refs.Resolved
	// PurgeAttachments removes the attachments of a deleted owner.
	PurgeAttachments func(ctx context.Context, ownerType string, ownerID int64)
	// Template looks a template up (templates belong to automation); journal.ErrNotFound when there is none.
	Template func(ctx context.Context, id int64) (TemplateRef, error)
}

// TemplateRef is what the journal needs to know about a template: its name and its tags.
type TemplateRef struct {
	Title string
	Tags  []journal.Tag
}

// Register mounts every route of the context.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/categories", h.listCategories)
	mux.HandleFunc("GET /api/quests", h.listQuests)
	mux.HandleFunc("POST /api/quests", h.createQuest)
	mux.HandleFunc("GET /api/quests/{id}", h.getQuest)
	mux.HandleFunc("PATCH /api/quests/{id}", h.patchQuest)
	mux.HandleFunc("DELETE /api/quests/{id}", h.deleteQuest)
	mux.HandleFunc("POST /api/quests/{id}/steps", h.addStep)
	mux.HandleFunc("PATCH /api/quests/{id}/steps/{step_id}", h.patchStep)
	mux.HandleFunc("DELETE /api/quests/{id}/steps/{step_id}", h.deleteStep)

	mux.HandleFunc("GET /api/tags", h.listTags)
	mux.HandleFunc("POST /api/tags", h.createTag)
	mux.HandleFunc("GET /api/tags/{id}", h.getTag)
	mux.HandleFunc("PATCH /api/tags/{id}", h.patchTag)
	mux.HandleFunc("DELETE /api/tags/{id}", h.deleteTag)
	mux.HandleFunc("PUT /api/quests/{id}/tags", h.putQuestTags)
	mux.HandleFunc("PUT /api/templates/{id}/tags", h.putTemplateTags)

	mux.HandleFunc("GET /api/questlines", h.listQuestlines)
	mux.HandleFunc("POST /api/questlines", h.createQuestline)
	mux.HandleFunc("GET /api/questlines/{id}", h.getQuestline)
	mux.HandleFunc("PATCH /api/questlines/{id}", h.patchQuestline)
	mux.HandleFunc("DELETE /api/questlines/{id}", h.deleteQuestline)
	mux.HandleFunc("GET /api/questlines/{id}/icon", h.getQuestlineIcon)
	mux.HandleFunc("POST /api/questlines/{id}/icon", h.postQuestlineIcon)
	mux.HandleFunc("DELETE /api/questlines/{id}/icon", h.deleteQuestlineIcon)

	mux.HandleFunc("GET /api/quest-log", h.getQuestLog)
}

var (
	writeJSON = httpx.WriteJSON
)

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}

// publishOpts reads how a change is announced: quietly (no toast) and from which client.
func publishOpts(r *http.Request) (quiet bool, source string) {
	quiet = events.QuietFromQuery(r.URL.Query().Get("quiet"))
	source = r.URL.Query().Get("source")
	if source == "" {
		// Clients label themselves in X-Quests-Source (web, mcp, cli…); the web UI tells its own edits apart
		// from everyone else's by it.
		if v := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Quests-Source"))); httpx.SourceLabelOK(v) {
			source = v
		}
	}
	source = events.SourceLabel(quiet, source)
	return quiet, source
}

// publishSilent wakes journal tabs without a HUD toast or sound.
func (h *Handler) publishSilent(ctx context.Context, kind, title string, extra map[string]any) {
	if h.Hub != nil {
		h.Hub.PublishSilent(wsctx.From(ctx), kind, title, extra)
	}
}

// Owner kinds of attachments that hang on journal entities.
const (
	ownerQuest     = "quest"
	ownerQuestline = "questline"
)
