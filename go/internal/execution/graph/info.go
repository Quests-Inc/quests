package graph

// Kind groups node types by what they do; the validator wants a source and a sink in every graph.
type Kind string

const (
	KindSource  Kind = "source"
	KindCode    Kind = "code"
	KindParse   Kind = "parse"
	KindControl Kind = "control"
	KindSink    Kind = "sink"
	// KindCustom is a node the workspace's owner made (quest=458): baked from other nodes or written as code. It is
	// expanded into its own nodes before a run, so the evaluator never sees it.
	KindCustom Kind = "custom"
)

// NodeInfo is what an editor needs to draw a node type.
type NodeInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Kind    Kind   `json:"kind"`
	Trusted bool   `json:"trusted"`
	// Description and Fields: of a custom node; Fields are what the inspector offers to edit.
	Description string        `json:"description,omitempty"`
	Fields      []CustomField `json:"fields,omitempty"`
}
