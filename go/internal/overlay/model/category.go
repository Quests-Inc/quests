package model

import (
	"strings"

	"github.com/quests-inc/quests/go/internal/overlay/api"
)

// CycleCategory returns the next/prev category slug by delta (±1).
func CycleCategory(cats []api.Category, current string, delta int) string {
	options := make([]string, 0, len(cats))
	for _, c := range cats {
		s := strings.TrimSpace(c.Slug)
		if s != "" {
			options = append(options, s)
		}
	}
	if len(options) == 0 {
		return ""
	}
	idx := 0
	want := strings.ToLower(strings.TrimSpace(current))
	for i, s := range options {
		if strings.ToLower(s) == want {
			idx = i
			break
		}
	}
	n := len(options)
	idx = (idx + delta) % n
	if idx < 0 {
		idx += n
	}
	return options[idx]
}
