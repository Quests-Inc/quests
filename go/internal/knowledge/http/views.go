package knowledgehttp

import (
	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/refs"
)

// Response shapes of the knowledge endpoints (the ones the web client needs types for).

// NoteVerdict is the outcome of a note's check, shown on every note.
type NoteVerdict struct {
	Outdated       bool   `json:"outdated"`
	OutdatedReason string `json:"outdated_reason"`
	HasCheck       bool   `json:"has_check"`
}

// NoteListItem is a note in a list: its fields, the verdict of its check, and — in a popularity sort — why it ranks where it does.
type NoteListItem struct {
	knowledge.NoteRead `tstype:",extends"`
	NoteVerdict        `tstype:",extends"`
	Popularity         *knowledge.NotePopularity `json:"popularity,omitempty"`
}

// Backlink is something that mentions a note.
type Backlink struct {
	Kind  string `json:"kind"`
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// NotePayload is a note read on its own: the list item plus its surroundings.
type NotePayload struct {
	NoteListItem `tstype:",extends"`
	Children     []knowledge.NoteBriefRead `json:"children"`
	Refs         []refs.Resolved           `json:"refs"`
	Backlinks    []Backlink                `json:"backlinks"`
	Check        NoteCheckView             `json:"check"`
	// Attachments is set by the context endpoint only.
	Attachments any `json:"attachments,omitempty"`
}

// NoteCheckView is a note's check as the API shows it.
type NoteCheckView struct {
	Command         *string `json:"command"`
	IntervalSeconds *int    `json:"interval_seconds"`
	RunOn           *string `json:"run_on"`
	LastRunAt       *string `json:"last_run_at"`
	Outdated        bool    `json:"outdated"`
	OutdatedAt      *string `json:"outdated_at"`
	Reason          string  `json:"reason"`
	Error           string  `json:"error"`
	ErrorAt         *string `json:"error_at"`
}
