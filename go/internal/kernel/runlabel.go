package kernel

import (
	"errors"
	"regexp"
	"strings"
)

// ErrBadRunnerLabel is the validation message for a runner label.
var ErrBadRunnerLabel = errors.New("label: 1-32 chars of a-z, 0-9, _ or -")

var runnerLabelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidRunnerLabel reports whether s is a well-formed runner label.
func ValidRunnerLabel(s string) bool { return runnerLabelRe.MatchString(s) }

// ValidRunOn: nil (run here) or a well-formed runner label.
func ValidRunOn(v *string) bool { return v == nil || runnerLabelRe.MatchString(*v) }

// NormalizeRunOn trims and lower-cases a runner label; empty means "here".
func NormalizeRunOn(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.ToLower(strings.TrimSpace(*v))
	if s == "" {
		return nil
	}
	return &s
}
