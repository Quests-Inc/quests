package graph

import "encoding/json"

// The kinds of ending a Code node's job can have other than a clean exit. They travel on the `failed` output as a JSON
// document, so a graph tells them apart by the `kind` field instead of by a port of its own.
const (
	FailExit        = "failed"      // the command exited with a non-zero code
	FailTimeout     = "timeout"     // it ran longer than its limit
	FailUnreachable = "unreachable" // no runner picked the job up in time
	FailAbandoned   = "abandoned"   // the runner vanished in the middle of the run: the result is unknown
	FailCanceled    = "canceled"    // somebody cancelled it by hand
)

// Failure is what the `failed` output carries: the kind, a message in words, and the details the kind has.
type Failure struct {
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// Outcome is the Code node's ending that leaves by the `failed` output.
func (f Failure) Outcome() CodeOutcome {
	b, _ := json.Marshal(f)
	return CodeOutcome{Port: "failed", Text: string(b)}
}
