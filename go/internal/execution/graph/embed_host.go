package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type embedHost struct {
	args   map[string]string
	result *string
	notes  NoteReader
}

func (h *embedHost) Source(ctx context.Context, n Node) (map[string]any, error) {
	if n.Type == "source.note" && h.notes != nil {
		var p SourceParams
		if err := strictParams(n.Params, &p); err != nil {
			return nil, err
		}
		title, description, err := h.notes(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"title": title, "description": description}, nil
	}
	if n.Type != "args" {
		return nil, fmt.Errorf("источник %s здесь не работает", n.Type)
	}
	var p struct {
		Params []ArgParam `json:"params"`
	}
	_ = json.Unmarshal(n.Params, &p)
	out := map[string]any{}
	for _, a := range p.Params {
		raw, ok := h.args[a.Name]
		if !ok {
			continue
		}
		switch a.Type {
		case TStr:
			out[a.Name] = raw
		case TInt:
			v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не целое число", a.Name)
			}
			out[a.Name] = v
		case TFloat:
			v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не число", a.Name)
			}
			out[a.Name] = v
		case TBool:
			v, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не да/нет", a.Name)
			}
			out[a.Name] = v
		}
	}
	return out, nil
}

func (h *embedHost) Sink(_ context.Context, n Node, in map[string]Token) error {
	if n.Type != "sink.result" {
		return fmt.Errorf("приёмник %s здесь не работает", n.Type)
	}
	s, err := ResultText(in["value"])
	if err != nil {
		return err
	}
	if len(s) > MaxTextBytes {
		return fmt.Errorf("результат больше 64 КБ")
	}
	*h.result = s
	return nil
}

// NoteReader reads raw source Markdown only; it must enforce the caller's workspace and privacy policy.
type NoteReader func(context.Context, int64) (string, string, error)

func firstReader(readers []NoteReader) NoteReader {
	if len(readers) > 0 {
		return readers[0]
	}
	return nil
}
