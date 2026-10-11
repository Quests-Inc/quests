package graphdraft

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/clamav"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/store"
)

// The relations map is edited through a draft: an ordered log of operations
// kept on the server. Nothing in it touches real data until /commit, and any
// single operation can be dropped without losing the rest.

// Handler serves the map draft endpoints: edits to the relations map kept on the server until committed.
type Handler struct {
	Store          *store.Store
	WebDAV         *webdav.Client
	ClamAV         *clamav.Client
	MaxUploadBytes int64
	// API is the whole HTTP API: a commit applies every pending op by calling it, so the usual rules and change log apply.
	API func() http.Handler
	// WebDAVOK, ClamAVOK and CheckQuota are the attachment upload gates of the knowledge context.
	WebDAVOK   func() bool
	ClamAVOK   func() bool
	CheckQuota func(w http.ResponseWriter, ctx context.Context, incoming int64) bool
}

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}

var draftKinds = map[string]bool{
	"create_quest":     true,
	"create_note":      true,
	"create_questline": true,
	"create_step":      true,
	"upload_file":      true,
	"add_ref":          true,
	"remove_ref":       true,
	"attach":           true,
	"edit_quest":       true,
	"edit_step":        true,
	"edit_note":        true,
	"set_parent":       true,
	"copy":             true,
}

// draftRef is "type:id" where id is a number (real entity) or "t<op>" (made
// by an op of this draft).
var draftRefRe = regexp.MustCompile(`^(quest|note|step|attachment|questline):(\d+|t\d+)$`)

type draftRef struct {
	Type string
	ID   string
}

func parseDraftRef(s string) (draftRef, bool) {
	m := draftRefRe.FindStringSubmatch(s)
	if m == nil {
		return draftRef{}, false
	}
	return draftRef{Type: m[1], ID: m[2]}, true
}

func (r draftRef) isTmp() bool { return strings.HasPrefix(r.ID, "t") }

func (r draftRef) String() string {
	return r.Type + ":" + r.ID
}

// payload shapes
type createPayload struct {
	Fields map[string]any `json:"fields"`
	// Quest owns a step (create_step); Owner owns an uploaded file.
	Quest string `json:"quest,omitempty"`
}

type filePayload struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Mime     string `json:"mime"`
	Declared string `json:"declared,omitempty"`
	Path     string `json:"webdav_path"`
	Owner    string `json:"owner,omitempty"`
}

type editPayload struct {
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields"`
}

// parentPayload moves a note under another note (Parent empty = to the top).
type parentPayload struct {
	ID     string `json:"id"`
	Parent string `json:"parent"`
	Was    string `json:"was"`
}

type copyPayload struct {
	Source string `json:"source"`
}

type linkPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type attachPayload struct {
	Item  string `json:"item"`
	Owner string `json:"owner"`
}

// Register mounts the map draft routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph-draft", h.getGraphDraft)
	mux.HandleFunc("POST /api/graph-draft/ops", h.postGraphDraftOp)
	mux.HandleFunc("PUT /api/graph-draft/ops/{id}", h.putGraphDraftOp)
	mux.HandleFunc("DELETE /api/graph-draft/ops/{id}", h.deleteGraphDraftOp)
	mux.HandleFunc("PUT /api/graph-draft/roots", h.putGraphDraftRoots)
	mux.HandleFunc("POST /api/graph-draft/files", h.postGraphDraftFile)
	mux.HandleFunc("DELETE /api/graph-draft", h.cancelGraphDraft)
	mux.HandleFunc("POST /api/graph-draft/commit", h.commitGraphDraft)
}

func (h *Handler) getGraphDraft(w http.ResponseWriter, r *http.Request) {
	ops, err := h.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	roots, _ := h.Store.DraftRoots(r.Context())
	writeJSON(w, 200, GraphDraftView{Ops: ops, Roots: roots})
}

func (h *Handler) postGraphDraftOp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if !draftKinds[in.Kind] || in.Kind == "upload_file" {
		writeErr(w, 422, "unknown op kind")
		return
	}
	ops, err := h.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if msg := h.validateDraftOp(r.Context(), in.Kind, in.Payload, ops); msg != "" {
		writeErr(w, 422, msg)
		return
	}
	op, err := h.Store.AddDraftOp(r.Context(), in.Kind, in.Payload)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, op)
}

func (h *Handler) deleteGraphDraftOp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	ops, err := h.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	found := false
	for _, o := range ops {
		found = found || o.ID == id
	}
	if !found {
		writeErr(w, 404, "op not found")
		return
	}
	ids := dependents(ops, id)
	if r.URL.Query().Get("dry") != "" {
		writeJSON(w, 200, map[string]any{"would_remove": ids})
		return
	}
	h.dropStagedFiles(r.Context(), ops, ids)
	if err := h.Store.DeleteDraftOps(r.Context(), ids); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"removed": ids})
}

func (h *Handler) putGraphDraftRoots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Roots []struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		} `json:"roots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	for _, x := range in.Roots {
		if x.Type != "quest" && x.Type != "note" && x.Type != "questline" || x.ID <= 0 {
			writeErr(w, 422, "bad root")
			return
		}
	}
	raw, _ := json.Marshal(in.Roots)
	if in.Roots == nil {
		raw = []byte("[]")
	}
	if err := h.Store.SetDraftRoots(r.Context(), raw); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"roots": json.RawMessage(raw)})
}

func (h *Handler) cancelGraphDraft(w http.ResponseWriter, r *http.Request) {
	ops, err := h.Store.ClearDraft(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids := make([]int64, len(ops))
	for i, o := range ops {
		ids[i] = o.ID
	}
	h.dropStagedFiles(r.Context(), ops, ids)
	w.WriteHeader(204)
}
