package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Limits of a stored graph.
const (
	MaxBytes = 512 << 10
	MaxNodes = 200
	MaxEdges = 1000
)

// Graph is the stored form of a script.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is one block. ID is a per-script sequence number given when the node is created and never reused; the editor's
// position is kept next to it.
type Node struct {
	ID     int             `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params,omitempty"`
	X      float64         `json:"x,omitempty"`
	Y      float64         `json:"y,omitempty"`
}

// End is one end of a link.
type End struct {
	Node int    `json:"node"`
	Port string `json:"port"`
}

// Edge is a link from an output to an input. Alias and Optional matter on a link into the `in` input of a Code node: they
// name the environment variable and say whether the node may run without that value.
type Edge struct {
	From     End    `json:"from"`
	To       End    `json:"to"`
	Alias    string `json:"alias,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// Decode reads a stored graph: size-capped, unknown fields refused.
func Decode(data []byte) (Graph, error) {
	if len(data) > MaxBytes {
		return Graph{}, fmt.Errorf("граф больше %d КБ", MaxBytes>>10)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var g Graph
	if err := dec.Decode(&g); err != nil {
		return Graph{}, fmt.Errorf("граф: %w", err)
	}
	if dec.More() {
		return Graph{}, errors.New("граф: после JSON есть лишний текст")
	}
	return g, nil
}

// EnvName is the environment variable a link into Code delivers its value as: the alias, or the number of the node the
// value comes from and the name of its port, so values from different nodes never overlap.
func EnvName(e Edge) string {
	if e.Alias != "" {
		return "QUESTS_IN_" + e.Alias
	}
	return fmt.Sprintf("QUESTS_IN_N%d_%s", e.From.Node, strings.ToUpper(e.From.Port))
}

const codeType = "code"

// RunsOnRunner reports whether the graph has a Code node: only then a runner is needed. A graph without one runs on the
// server alone, at once.
func (g Graph) RunsOnRunner() bool {
	for _, n := range g.Nodes {
		if n.Type == codeType {
			return true
		}
	}
	return false
}

// Subjects lists the kinds of entity the graph's sources read (`step`, `note`…), so a script can be offered only where it
// fits.
func (g Graph) Subjects() []string {
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if kind, ok := strings.CutPrefix(n.Type, "source."); ok {
			seen[kind] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WithoutLayout is the graph with the editor's positions removed: what is left is what the script does, so two graphs that
// differ only in where their nodes sit compare equal.
func (g Graph) WithoutLayout() Graph {
	nodes := make([]Node, len(g.Nodes))
	for i, n := range g.Nodes {
		n.X, n.Y = 0, 0
		nodes[i] = n
	}
	return Graph{Nodes: nodes, Edges: g.Edges}
}

// Structure is the graph's skeleton: which nodes of which types are joined how. Parameters (the text of a Code node, a
// Compare's operand) and positions are left out, so a script an owner only tuned still has the skeleton of its sample.
func (g Graph) Structure() string {
	parts := make([]string, 0, len(g.Nodes)+len(g.Edges))
	for _, n := range g.Nodes {
		parts = append(parts, fmt.Sprintf("n%d:%s", n.ID, n.Type))
	}
	for _, e := range g.Edges {
		parts = append(parts, fmt.Sprintf("e%d.%s>%d.%s|%s|%t", e.From.Node, e.From.Port, e.To.Node, e.To.Port, e.Alias, e.Optional))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}
