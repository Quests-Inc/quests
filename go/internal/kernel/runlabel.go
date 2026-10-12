package kernel

import (
	"errors"
	"regexp"
	"strings"
)

// ErrBadRunnerLabel is the validation message for a runner label.
var ErrBadRunnerLabel = errors.New("label: 1-32 chars of a-z, 0-9, _ or -")

// ErrBadRunOn is the validation message for a `run_on` list.
var ErrBadRunOn = errors.New("run_on: runner labels (1-32 chars of a-z, 0-9, _ or -), at most 8, comma-separated, no repeats")

// MaxRunOnLabels caps the fallback list of one command.
const MaxRunOnLabels = 8

var runnerLabelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidRunnerLabel reports whether s is a well-formed runner label.
func ValidRunnerLabel(s string) bool { return runnerLabelRe.MatchString(s) }

// SplitRunOn lists the labels of a stored `run_on` in the order they are tried.
func SplitRunOn(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ValidRunOn: nil (run here) or a list of well-formed, distinct runner labels (a single one is the common case).
// With several, the command goes to the first that is online and falls to the next when it does not answer;
// it is up to the author that the command does not depend on one particular device.
func ValidRunOn(v *string) bool {
	if v == nil {
		return true
	}
	labels := strings.Split(*v, ",")
	if len(labels) > MaxRunOnLabels {
		return false
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if !runnerLabelRe.MatchString(l) || seen[l] {
			return false
		}
		seen[l] = true
	}
	return true
}

// NormalizeRunOn trims and lower-cases the labels, drops empty and repeated ones and joins them with a comma;
// empty means "here".
func NormalizeRunOn(v *string) *string {
	if v == nil {
		return nil
	}
	var kept []string
	seen := map[string]bool{}
	for _, l := range SplitRunOn(strings.ToLower(*v)) {
		if !seen[l] {
			seen[l] = true
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	s := strings.Join(kept, ",")
	return &s
}
