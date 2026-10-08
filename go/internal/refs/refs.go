package refs

import (
	"regexp"
	"strconv"
)

// Kind IDs agents already paste: quest=23, note=12, …
var TokenRe = regexp.MustCompile(`\b(note|quest|questline|step|attachment|template)=(\d+)\b`)

type Ref struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

func Parse(text string) []Ref {
	if text == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []Ref
	for _, m := range TokenRe.FindAllStringSubmatch(text, -1) {
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		key := m[1] + "=" + m[2]
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Ref{Kind: m[1], ID: id})
	}
	return out
}

func FilterKind(list []Ref, kind string) []Ref {
	var out []Ref
	for _, r := range list {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// Rewrite replaces kind=ID tokens through resolve, which returns the full
// replacement token (or false to leave a token as it is). Same grammar as Parse,
// so whatever the app would show as a link is what gets rewritten.
func Rewrite(text string, resolve func(kind string, id int64) (string, bool)) string {
	if text == "" {
		return text
	}
	return TokenRe.ReplaceAllStringFunc(text, func(tok string) string {
		m := TokenRe.FindStringSubmatch(tok)
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil || id <= 0 {
			return tok
		}
		if repl, ok := resolve(m[1], id); ok {
			return repl
		}
		return tok
	})
}
