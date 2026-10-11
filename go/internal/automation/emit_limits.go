package automation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// EmitLimits bounds what an emit_pool_command may put into the quest it
// returns. Defaults suit most templates; a template can override any of them
// (questtemplate.emit_limits, a JSON object) without a backend change.
type EmitLimits struct {
	MaxSteps       int `json:"max_steps"`
	MaxTitle       int `json:"max_title"`
	MaxDescription int `json:"max_description"`
	MaxCommand     int `json:"max_command"`
}

// DefaultEmitLimits are the limits used when a template sets none.
func DefaultEmitLimits() EmitLimits {
	return EmitLimits{MaxSteps: 30, MaxTitle: 200, MaxDescription: 20000, MaxCommand: 2000}
}

// ParseEmitLimits reads a template's emit_limits JSON over the defaults.
// Empty means "defaults". Unknown keys and non-positive values are errors (a
// typo must not silently leave a limit at its default).
func ParseEmitLimits(raw string) (EmitLimits, error) {
	lim := DefaultEmitLimits()
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return lim, nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var in struct {
		MaxSteps       *int `json:"max_steps"`
		MaxTitle       *int `json:"max_title"`
		MaxDescription *int `json:"max_description"`
		MaxCommand     *int `json:"max_command"`
	}
	if err := dec.Decode(&in); err != nil {
		return lim, fmt.Errorf("emit_limits: %v (allowed keys: max_steps, max_title, max_description, max_command)", err)
	}
	for name, p := range map[string]*int{"max_steps": in.MaxSteps, "max_title": in.MaxTitle, "max_description": in.MaxDescription, "max_command": in.MaxCommand} {
		if p != nil && *p < 1 {
			return lim, fmt.Errorf("emit_limits: %s must be a positive integer", name)
		}
	}
	if in.MaxSteps != nil {
		lim.MaxSteps = *in.MaxSteps
	}
	if in.MaxTitle != nil {
		lim.MaxTitle = *in.MaxTitle
	}
	if in.MaxDescription != nil {
		lim.MaxDescription = *in.MaxDescription
	}
	if in.MaxCommand != nil {
		lim.MaxCommand = *in.MaxCommand
	}
	return lim, nil
}

// ResolveQuestlineRef maps a questline given as a numeric id or as a
// name/substring to its id: an exact (case-insensitive) title match wins over a
// substring, and an ambiguous name is an error.
func (s *Store) ResolveQuestlineRef(ctx context.Context, raw string) (int64, error) {
	return s.resolveNamed(ctx, "questline", raw, `SELECT id, title FROM questline`)
}

// ResolveCategoryRef is ResolveQuestlineRef for categories (slug or label).
func (s *Store) ResolveCategoryRef(ctx context.Context, raw string) (int64, error) {
	return s.resolveNamed(ctx, "category", raw, `SELECT id, label, slug FROM questcategory`)
}

func (s *Store) resolveNamed(ctx context.Context, what, raw, query string) (int64, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, fmt.Errorf("%s: empty value", what)
	}
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	type row struct {
		id    int64
		names []string
	}
	var all []row
	for rows.Next() {
		vals := make([]sql.NullString, len(cols)-1)
		var id int64
		dest := []any{&id}
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return 0, err
		}
		r := row{id: id}
		for _, v := range vals {
			if v.Valid {
				r.names = append(r.names, v.String)
			}
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var asID int64
	if _, err := fmt.Sscan(text, &asID); err == nil && fmt.Sprintf("%d", asID) == text {
		for _, r := range all {
			if r.id == asID {
				return asID, nil
			}
		}
		return 0, fmt.Errorf("%s %d not found", what, asID)
	}
	needle := strings.ToLower(text)
	var exact, partial []int64
	for _, r := range all {
		for _, n := range r.names {
			ln := strings.ToLower(n)
			if ln == needle {
				exact = append(exact, r.id)
				break
			}
			if strings.Contains(ln, needle) {
				partial = append(partial, r.id)
				break
			}
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0], nil
	case len(exact) > 1:
		return 0, fmt.Errorf("%s %q is ambiguous (%d exact matches)", what, text, len(exact))
	case len(partial) == 1:
		return partial[0], nil
	case len(partial) > 1:
		return 0, fmt.Errorf("%s %q is ambiguous (%d matches)", what, text, len(partial))
	}
	return 0, fmt.Errorf("%s %q not found", what, text)
}
