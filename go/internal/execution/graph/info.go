package graph

// Kind groups node types by what they do; the validator wants a source and a sink in every graph.
type Kind string

const (
	KindSource  Kind = "source"
	KindCode    Kind = "code"
	KindParse   Kind = "parse"
	KindControl Kind = "control"
	KindSink    Kind = "sink"
)

// NodeInfo is what an editor needs to draw a node type.
type NodeInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Kind    Kind   `json:"kind"`
	Trusted bool   `json:"trusted"`
}
