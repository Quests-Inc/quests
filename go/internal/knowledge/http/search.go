package knowledgehttp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

// searchNotes ranks notes against a query: stemmed words in Russian and English, prefixes, the
// other keyboard layout, typos in titles. Each hit says where it matched and carries a fragment
// with the matches marked (⟪ ⟫). An agent (MCP) is never shown PRIVATE notes.
func (h *Handler) searchNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	opts := knowledge.SearchOpts{Limit: limit}
	if src := requestSource(r); src != nil && *src == "mcp" {
		opts.ExcludePrivate = true
		opts.Agent = true
	}
	want := parseNoteFlagFilter(q.Get("flags"))
	if want.bad != "" {
		writeErr(w, http.StatusUnprocessableEntity, "unknown flag: "+want.bad+" (known: "+noteFlagNames+")")
		return
	}
	for _, v := range want.include {
		opts.Filters = append(opts.Filters, search.Filter{Key: "flag", Values: []string{v}})
	}
	for _, v := range want.exclude {
		opts.Filters = append(opts.Filters, search.Filter{Key: "flag", Values: []string{v}, Not: true})
	}
	hits, err := h.Store.SearchNotes(r.Context(), q.Get("q"), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"q": strings.TrimSpace(q.Get("q")), "hits": hits, "marks": []string{knowledge.MarkOpen, knowledge.MarkClose}})
}

// searchInNote finds a query inside one note with the same rules as the notes search; the answer is
// the matched words (to mark them in the text) and the number of paragraphs that carry them.
func (h *Handler) searchInNote(w http.ResponseWriter, r *http.Request) {
	_, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	if src := requestSource(r); src != nil && *src == "mcp" && n.IsPrivate {
		writeErr(w, 404, "Note not found")
		return
	}
	res, err := h.Store.SearchInNote(r.Context(), n.Description, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// noteFlagNames are the flags a note search can filter on (flags=outdated,-private,…).
const noteFlagNames = "outdated|stale, readme, private, category, pinned|pin, check"

type noteFlagFilter struct {
	include, exclude []string
	bad              string
}

func (f noteFlagFilter) active() bool { return len(f.include)+len(f.exclude) > 0 }

func canonNoteFlag(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "outdated", "stale":
		return "outdated"
	case "readme", "private", "category", "check":
		return strings.ToLower(strings.TrimSpace(name))
	case "pinned", "pin":
		return "pinned"
	}
	return ""
}

// parseNoteFlagFilter reads "outdated,-private" (a minus means "without the flag").
func parseNoteFlagFilter(raw string) noteFlagFilter {
	var f noteFlagFilter
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		neg := strings.HasPrefix(part, "-")
		name := canonNoteFlag(strings.TrimPrefix(part, "-"))
		if name == "" {
			f.bad = part
			return f
		}
		if neg {
			f.exclude = append(f.exclude, name)
		} else {
			f.include = append(f.include, name)
		}
	}
	return f
}
