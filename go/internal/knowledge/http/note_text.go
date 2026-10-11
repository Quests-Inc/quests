package knowledgehttp

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	errNoteTextAnchorMissing = errors.New("anchor not found in note description")
	errNoteTextFindMissing   = errors.New("find text not found in note description")
	errNoteTextAtRange       = errors.New("at is out of range")
	errNoteTextEmptyFind     = errors.New("find must be non-empty")
	errNoteTextEmptyText     = errors.New("text must be non-empty")
	errNoteTextBadPos        = errors.New("provide at most one of after, before, at")
)

// applyDescriptionInsert inserts text into src. Position: after/before the
// first occurrence of an anchor, at a rune offset, or append when none given.
func applyDescriptionInsert(src, text string, after, before *string, at *int) (string, error) {
	if text == "" {
		return "", errNoteTextEmptyText
	}
	nPos := 0
	if after != nil {
		nPos++
	}
	if before != nil {
		nPos++
	}
	if at != nil {
		nPos++
	}
	if nPos > 1 {
		return "", errNoteTextBadPos
	}

	runes := []rune(src)
	var idx int
	switch {
	case after != nil:
		a := *after
		if a == "" {
			return "", errNoteTextEmptyFind
		}
		i := strings.Index(src, a)
		if i < 0 {
			return "", errNoteTextAnchorMissing
		}
		idx = utf8.RuneCountInString(src[:i+len(a)])
	case before != nil:
		b := *before
		if b == "" {
			return "", errNoteTextEmptyFind
		}
		i := strings.Index(src, b)
		if i < 0 {
			return "", errNoteTextAnchorMissing
		}
		idx = utf8.RuneCountInString(src[:i])
	case at != nil:
		if *at < 0 || *at > len(runes) {
			return "", fmt.Errorf("%w (0..%d)", errNoteTextAtRange, len(runes))
		}
		idx = *at
	default:
		idx = len(runes)
	}

	ins := []rune(text)
	out := make([]rune, 0, len(runes)+len(ins))
	out = append(out, runes[:idx]...)
	out = append(out, ins...)
	out = append(out, runes[idx:]...)
	return string(out), nil
}

// applyDescriptionReplace replaces find with withText. all=false → first only.
func applyDescriptionReplace(src, find, withText string, all bool) (string, error) {
	if find == "" {
		return "", errNoteTextEmptyFind
	}
	if !strings.Contains(src, find) {
		return "", errNoteTextFindMissing
	}
	if all {
		return strings.ReplaceAll(src, find, withText), nil
	}
	return strings.Replace(src, find, withText, 1), nil
}
