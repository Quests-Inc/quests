package search

import (
	"regexp"
	"strings"
)

// A note's own variable: `var:name` or `var:name=value`, and `-var:` to exclude.
// The token is taken out of the text query so it is not ranked as words.
var varToken = regexp.MustCompile(`(?i)(^|\s)(-?)var:(\S+)`)

// NormalizeNoteVars moves var: tokens from the query into note-only filters.
// A filter that already uses key var is limited to notes, so other types stay searchable.
func NormalizeNoteVars(r Request) Request {
	text, extra := takeVarFilters(r.Query)
	r.Query = text
	filters := make([]Filter, len(r.Filters), len(r.Filters)+len(extra))
	copy(filters, r.Filters)
	for i := range filters {
		if filters[i].Key == "var" && filters[i].Kind == "" {
			filters[i].Kind = "note"
		}
	}
	r.Filters = append(filters, extra...)
	return r
}

// HasVarFilter reports a note-variable constraint, including one just parsed from the query.
func HasVarFilter(filters []Filter) bool {
	for _, f := range filters {
		if f.Key == "var" && len(f.Values) > 0 {
			return true
		}
	}
	return false
}

func takeVarFilters(q string) (string, []Filter) {
	var pos, neg []string
	text := varToken.ReplaceAllStringFunc(q, func(m string) string {
		sub := varToken.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		spec := sub[3]
		if spec == "" || strings.HasPrefix(spec, "=") || strings.HasPrefix(spec, "= ") {
			return m
		}
		if name, _, _ := splitVarSpec(spec); name == "" {
			return m
		}
		if sub[2] == "-" {
			neg = append(neg, spec)
		} else {
			pos = append(pos, spec)
		}
		return sub[1]
	})
	text = strings.Join(strings.Fields(text), " ")
	var filters []Filter
	if len(pos) > 0 {
		filters = append(filters, Filter{Kind: "note", Key: "var", Values: pos})
	}
	for _, spec := range neg {
		filters = append(filters, Filter{Kind: "note", Key: "var", Values: []string{spec}, Not: true})
	}
	return text, filters
}

// splitVarSpec splits `name` or `name=value` on the first equals sign.
func splitVarSpec(spec string) (name, value string, exact bool) {
	name, value, exact = strings.Cut(spec, "=")
	return name, value, exact
}
