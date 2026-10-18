package graph

import (
	"encoding/json"
	"fmt"
)

// Snapshot is a run frozen between a Code node's request and its outcome: the host stores it as JSON with the job and
// restores it when the outcome arrives, so nothing in the process has to survive a restart.
type Snapshot struct {
	States   map[int]NodeState   `json:"states"`
	Failures map[int]string      `json:"failures,omitempty"`
	Outcomes map[int]CodeOutcome `json:"outcomes,omitempty"`
	Tokens   []SnapshotToken     `json:"tokens"`
}

// SnapshotToken is one token of a link, by the output it came from.
type SnapshotToken struct {
	Node  int             `json:"node"`
	Port  string          `json:"port"`
	Exit  bool            `json:"exit,omitempty"`
	Type  PortType        `json:"type,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Snapshot freezes the run.
func (r *Run) Snapshot() (Snapshot, error) {
	s := Snapshot{States: map[int]NodeState{}, Failures: map[int]string{}, Outcomes: map[int]CodeOutcome{}}
	for k, v := range r.states {
		s.States[k] = v
	}
	for k, v := range r.failures {
		s.Failures[k] = v
	}
	for k, v := range r.outcomes {
		s.Outcomes[k] = v
	}
	for _, id := range r.order {
		for _, p := range r.nodes[id].outList {
			t, ok := r.tokens[End{id, p.Name}]
			if !ok {
				continue
			}
			st := SnapshotToken{Node: id, Port: p.Name, Exit: t.Exit, Type: t.Type}
			if !t.Exit {
				b, err := json.Marshal(t.Value)
				if err != nil {
					return Snapshot{}, fmt.Errorf("нода %d, выход %q: %w", id, p.Name, err)
				}
				st.Value = b
			}
			s.Tokens = append(s.Tokens, st)
		}
	}
	return s, nil
}

// Restore thaws a run of the same graph.
func Restore(g Graph, reg Registry, s Snapshot) (*Run, error) {
	r, err := NewRun(g, reg)
	if err != nil {
		return nil, err
	}
	for k, v := range s.States {
		r.states[k] = v
	}
	for k, v := range s.Failures {
		r.failures[k] = v
	}
	for k, v := range s.Outcomes {
		r.outcomes[k] = v
	}
	for _, t := range s.Tokens {
		if t.Exit {
			r.tokens[End{t.Node, t.Port}] = exitToken()
			continue
		}
		v, err := decodeValue(t.Type, t.Value)
		if err != nil {
			return nil, fmt.Errorf("нода %d, выход %q: %w", t.Node, t.Port, err)
		}
		r.tokens[End{t.Node, t.Port}] = Token{Type: t.Type, Value: v}
	}
	return r, nil
}

func decodeValue(t PortType, raw json.RawMessage) (any, error) {
	switch t {
	case TInt:
		var v int64
		return v, json.Unmarshal(raw, &v)
	case TFloat:
		var v float64
		return v, json.Unmarshal(raw, &v)
	case TStr:
		var v string
		return v, json.Unmarshal(raw, &v)
	case TBool:
		var v bool
		return v, json.Unmarshal(raw, &v)
	case TDoc:
		return json.RawMessage(raw), nil
	case TSecret:
		var v Secrets
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &v) // a snapshot from before the secrets named their entity holds {}
		}
		return v, nil
	case TRefStep, TRefNote, TRefTemplate, TRefQuest, TRefWorkspace:
		var v Ref
		return v, json.Unmarshal(raw, &v)
	case TAny:
		var v any
		return v, json.Unmarshal(raw, &v)
	}
	return nil, fmt.Errorf("неизвестный тип %q", t)
}
