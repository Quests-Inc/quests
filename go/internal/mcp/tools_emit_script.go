package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// emitScriptCommand finds what a template's emit script would run on the runner: the text of its Code node and the fixed
// variables (`vars`) it sets, so the dry run can try it here like it tries a plain emit_pool_command. A node made of code
// (custom.N) counts too, with the values its fields have in the script. Several Code nodes are ambiguous: say which.
func emitScriptCommand(ctx context.Context, c *Client, scriptID int64) (string, map[string]string, error) {
	var sc struct {
		Graph struct {
			Nodes []graphNode `json:"nodes"`
		} `json:"graph"`
	}
	if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/scripts/%d", scriptID), nil, nil, &sc); err != nil {
		return "", nil, err
	}
	type found struct {
		text string
		env  map[string]string
	}
	var codes []found
	for _, n := range sc.Graph.Nodes {
		switch {
		case n.Type == "code":
			codes = append(codes, found{codeText(n.Params), codeVars(n.Params)})
		case strings.HasPrefix(n.Type, "custom."):
			var def struct {
				Fields []struct {
					Name    string `json:"name"`
					Default any    `json:"default"`
					Node    int    `json:"node"`
					Param   string `json:"param"`
				} `json:"fields"`
				Graph struct {
					Nodes []graphNode `json:"nodes"`
				} `json:"graph"`
			}
			if err := c.Do(ctx, http.MethodGet, "/api/script-custom-nodes/"+strings.TrimPrefix(n.Type, "custom."), nil, nil, &def); err != nil {
				return "", nil, err
			}
			for _, in := range def.Graph.Nodes {
				if in.Type != "code" {
					continue
				}
				text, env := codeText(in.Params), codeVars(in.Params)
				for _, f := range def.Fields {
					if f.Node != in.ID {
						continue
					}
					v := f.Default
					if set, ok := n.Params[f.Name]; ok {
						v = set
					}
					if v == nil {
						continue
					}
					if name, isVar := strings.CutPrefix(f.Param, "vars."); isVar {
						env[name] = fmt.Sprint(v)
					} else if f.Param == "text" {
						text = fmt.Sprint(v)
					}
				}
				codes = append(codes, found{text, env})
			}
		}
	}
	switch len(codes) {
	case 0:
		return "", nil, &Err{422, fmt.Sprintf("script %d has no Code node to try", scriptID)}
	case 1:
		return codes[0].text, codes[0].env, nil
	}
	return "", nil, &Err{422, fmt.Sprintf("script %d has %d Code nodes: try one by passing its text as command", scriptID, len(codes))}
}

type graphNode struct {
	ID     int            `json:"id"`
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

func codeText(p map[string]any) string {
	s, _ := p["text"].(string)
	return s
}

func codeVars(p map[string]any) map[string]string {
	env := map[string]string{}
	if m, ok := p["vars"].(map[string]any); ok {
		for k, v := range m {
			env[k] = fmt.Sprint(v)
		}
	}
	return env
}
