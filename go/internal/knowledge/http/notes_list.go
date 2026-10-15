package knowledgehttp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/quests-inc/quests/go/internal/knowledge"
)

// Page sizes of the brief note list (GET /api/notes?brief=1), same bounds as the quest summary.
const (
	briefDefaultLimit = 20
	briefMaxLimit     = 100
)

// NoteBriefListItem is a note in a list without its text: the fields to pick one, and how long it is.
type NoteBriefListItem struct {
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	ParentID       *int64  `json:"parent_id"`
	Pinned         bool    `json:"pinned"`
	IsReadme       bool    `json:"is_readme"`
	IsPrivate      bool    `json:"is_private"`
	IsCategory     bool    `json:"is_category"`
	UpdatedAt      *string `json:"updated_at"`
	DescriptionLen int     `json:"description_len"`
	NoteVerdict
	Popularity *knowledge.NotePopularity `json:"popularity,omitempty"`
}

// NoteBriefPage is one page of the brief list. Next is the id of the last note of this page.
type NoteBriefPage struct {
	Items     []NoteBriefListItem `json:"items"`
	Total     int                 `json:"total"`
	Truncated bool                `json:"truncated"`
	Next      string              `json:"next,omitempty"`
}

var errBadNoteCursor = errors.New("invalid cursor")

// briefNotePage cuts a list (already in its final order) into a page; the cursor is the id of the last
// note handed out, so the tree order of the list is kept and nothing is skipped when notes are added later.
func briefNotePage(items []NoteListItem, limit int, cursor string) (NoteBriefPage, error) {
	if limit <= 0 {
		limit = briefDefaultLimit
	}
	if limit > briefMaxLimit {
		limit = briefMaxLimit
	}
	start := 0
	if cursor != "" {
		id, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			return NoteBriefPage{}, errBadNoteCursor
		}
		found := false
		for i, it := range items {
			if it.ID == id {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return NoteBriefPage{}, errBadNoteCursor
		}
	}
	page := NoteBriefPage{Total: len(items), Items: []NoteBriefListItem{}}
	for i := start; i < len(items); i++ {
		if len(page.Items) == limit {
			page.Truncated = true
			page.Next = strconv.FormatInt(page.Items[len(page.Items)-1].ID, 10)
			break
		}
		it := items[i]
		page.Items = append(page.Items, NoteBriefListItem{
			ID: it.ID, Title: it.Title, ParentID: it.ParentID, Pinned: it.Pinned, IsReadme: it.IsReadme,
			IsPrivate: it.IsPrivate, IsCategory: it.IsCategory, UpdatedAt: it.UpdatedAt,
			DescriptionLen: utf8.RuneCountInString(it.Description), NoteVerdict: it.NoteVerdict, Popularity: it.Popularity,
		})
	}
	return page, nil
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

// notesUnderCategory keeps the notes nested anywhere below a CATEGORY note (not the category itself).
func (h *Handler) notesUnderCategory(r *http.Request, rows []knowledge.Note, raw string) ([]knowledge.Note, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, httpError{400, "invalid category_id"}
	}
	all, err := h.Store.ListNotes(r.Context(), knowledge.NoteFilter{})
	if err != nil {
		return nil, err
	}
	children := map[int64][]int64{}
	var root *knowledge.Note
	for i, n := range all {
		if n.ID == id {
			root = &all[i]
		}
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	if root == nil {
		return nil, httpError{404, "Note not found"}
	}
	if !root.IsCategory {
		return nil, httpError{422, "note is not a CATEGORY note"}
	}
	under := map[int64]bool{}
	stack := append([]int64(nil), children[id]...)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if under[cur] {
			continue
		}
		under[cur] = true
		stack = append(stack, children[cur]...)
	}
	out := rows[:0:0]
	for _, n := range rows {
		if under[n.ID] {
			out = append(out, n)
		}
	}
	return out, nil
}

// noteFlags maps the flag names of ?flags= to what they test on a listed note.
var noteFlags = map[string]func(NoteListItem) bool{
	"outdated": func(n NoteListItem) bool { return n.Outdated },
	"stale":    func(n NoteListItem) bool { return n.Outdated },
	"readme":   func(n NoteListItem) bool { return n.IsReadme },
	"private":  func(n NoteListItem) bool { return n.IsPrivate },
	"category": func(n NoteListItem) bool { return n.IsCategory },
	"pinned":   func(n NoteListItem) bool { return n.Pinned },
	"pin":      func(n NoteListItem) bool { return n.Pinned },
	"check":    func(n NoteListItem) bool { return n.HasCheck },
}

// filterNoteFlags keeps the notes that have every flag of a comma list and none of the "-flag"s.
func filterNoteFlags(items []NoteListItem, raw string) ([]NoteListItem, error) {
	type want struct {
		test func(NoteListItem) bool
		neg  bool
	}
	var ws []want
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		neg := strings.HasPrefix(part, "-")
		test, ok := noteFlags[strings.ToLower(strings.TrimLeft(part, "-"))]
		if !ok {
			return nil, fmt.Errorf("unknown flag %q; known: outdated, readme, private, category, pinned, check", part)
		}
		ws = append(ws, want{test, neg})
	}
	out := make([]NoteListItem, 0, len(items))
next:
	for _, n := range items {
		for _, w := range ws {
			if w.test(n) == w.neg {
				continue next
			}
		}
		out = append(out, n)
	}
	return out, nil
}
