package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// scriptTools is one multi-action tool (like trash): the agent works with the script library
// without blowing the ≤50 tool budget (ADR-0003).
func scriptTools(c *Client) []Tool {
	type args struct {
		Action      string  `json:"action" enum:"list,get,create,update,delete,validate,nodes,samples,bind,unbind"`
		ID          int64   `json:"id,omitempty" desc:"script id (get/update/delete) or the step/note/template id (bind/unbind)"`
		Version     int     `json:"version,omitempty" desc:"get: a past graph version; update: the version you read (always required)"`
		Title       string  `json:"title,omitempty" desc:"create (required) / update"`
		Description *string `json:"description,omitempty" desc:"create/update"`
		Graph       any     `json:"graph,omitempty" desc:"create/update/validate: node graph (docs/adr/0003-script-graphs.md, docs/automation.md)"`
		Sample      string  `json:"sample,omitempty" desc:"create: copy a sample id from action=samples instead of graph"`
		Subject     string  `json:"subject,omitempty" enum:"step,note,template" desc:"bind/unbind: what to attach the script to"`
		ScriptID    int64   `json:"script_id,omitempty" desc:"bind: which script"`
		Limit       int     `json:"limit,omitempty" desc:"list: default 20, max 100"`
	}
	s := Typed("script", "Workspace scripts (ADR-0003: graphs of typed nodes). Actions: list (summaries), get (with graph; version for a past save), create (graph or sample id from samples), update (pass version you read), delete, validate (check a graph without saving), nodes (catalog), samples (starters), bind / unbind (subject=step|note|template). Format: docs/adr/0003-script-graphs.md and docs/automation.md. Owner-only for create/update/delete/bind.",
		func(ctx context.Context, a args) (any, error) {
			switch a.Action {
			case "list":
				return scriptList(ctx, c, a.Limit)
			case "get":
				return scriptGet(ctx, c, a.ID, a.Version)
			case "create":
				return scriptCreate(ctx, c, a.Title, a.Description, a.Graph, a.Sample)
			case "update":
				return scriptUpdate(ctx, c, a.ID, a.Version, a.Title, a.Description, a.Graph)
			case "delete":
				if a.ID < 1 {
					return nil, &Err{422, "delete needs id"}
				}
				if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/scripts/%d", a.ID), nil, nil, nil); err != nil {
					return nil, err
				}
				return ok(ctx, c, a.ID, map[string]any{"deleted": true}), nil
			case "validate":
				return scriptValidate(ctx, c, a.Graph)
			case "nodes":
				var nodes []map[string]any
				if err := c.Do(ctx, http.MethodGet, "/api/script-nodes", nil, nil, &nodes); err != nil {
					return nil, err
				}
				return map[string]any{"items": nodes, "total": len(nodes), "workspace": c.Workspace(ctx)}, nil
			case "samples":
				return scriptSamples(ctx, c)
			case "bind":
				return scriptBind(ctx, c, a.Subject, a.ID, a.ScriptID)
			case "unbind":
				return scriptUnbind(ctx, c, a.Subject, a.ID)
			default:
				return nil, &Err{422, "action: list, get, create, update, delete, validate, nodes, samples, bind or unbind"}
			}
		})
	return []Tool{s}
}

func scriptList(ctx context.Context, c *Client, limit int) (any, error) {
	var rows []map[string]any
	if err := c.Do(ctx, http.MethodGet, "/api/scripts", nil, nil, &rows); err != nil {
		return nil, err
	}
	lim := clampLimit(limit, 20)
	items := make([]map[string]any, 0, min(lim, len(rows)))
	for i, r := range rows {
		if i == lim {
			break
		}
		items = append(items, map[string]any{
			"id": r["id"], "title": r["title"], "version": r["version"],
			"subjects": r["subjects"], "needs_runner": r["needs_runner"], "template": r["template"],
			"updated_at": r["updated_at"],
		})
	}
	return map[string]any{"items": items, "total": len(rows), "truncated": len(rows) > lim, "workspace": c.Workspace(ctx)}, nil
}

func scriptGet(ctx context.Context, c *Client, id int64, version int) (any, error) {
	if id < 1 {
		return nil, &Err{422, "get needs id"}
	}
	if version > 0 {
		var res map[string]any
		if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/scripts/%d/versions/%d", id, version), nil, nil, &res); err != nil {
			return nil, err
		}
		res["workspace"] = c.Workspace(ctx)
		return res, nil
	}
	var sc map[string]any
	if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/scripts/%d", id), nil, nil, &sc); err != nil {
		return nil, err
	}
	sc["workspace"] = c.Workspace(ctx)
	return sc, nil
}

func scriptCreate(ctx context.Context, c *Client, title string, description *string, graph any, sample string) (any, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, &Err{422, "create needs title"}
	}
	g := graph
	if strings.TrimSpace(sample) != "" {
		if g != nil {
			return nil, &Err{422, "create: give graph or sample, not both"}
		}
		var err error
		g, err = sampleGraph(ctx, c, sample)
		if err != nil {
			return nil, err
		}
	}
	if g == nil {
		return nil, &Err{422, "create needs graph or sample (see action=samples)"}
	}
	body := map[string]any{"title": title, "graph": g}
	if description != nil {
		body["description"] = *description
	}
	var sc map[string]any
	if err := c.Do(ctx, http.MethodPost, "/api/scripts", nil, body, &sc); err != nil {
		return nil, err
	}
	id, _ := sc["id"].(float64)
	return ok(ctx, c, int64(id), map[string]any{"version": sc["version"], "subjects": sc["subjects"], "needs_runner": sc["needs_runner"]}), nil
}

func sampleGraph(ctx context.Context, c *Client, id string) (any, error) {
	var samples []map[string]any
	if err := c.Do(ctx, http.MethodGet, "/api/script-samples", nil, nil, &samples); err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(id))
	for _, s := range samples {
		if strings.ToLower(str(s["id"])) == needle {
			return s["graph"], nil
		}
	}
	ids := make([]string, 0, len(samples))
	for _, s := range samples {
		ids = append(ids, str(s["id"]))
	}
	return nil, &Err{422, fmt.Sprintf("unknown sample %q; known: %s", id, strings.Join(ids, ", "))}
}

func scriptUpdate(ctx context.Context, c *Client, id int64, version int, title string, description *string, graph any) (any, error) {
	if id < 1 {
		return nil, &Err{422, "update needs id"}
	}
	if version < 1 {
		return nil, &Err{422, "update needs version (the one get returned)"}
	}
	body := map[string]any{"version": version}
	changed := false
	if strings.TrimSpace(title) != "" {
		body["title"] = title
		changed = true
	}
	if description != nil {
		body["description"] = *description
		changed = true
	}
	if graph != nil {
		body["graph"] = graph
		changed = true
	}
	if !changed {
		return nil, &Err{422, "provide title, description and/or graph to update"}
	}
	var sc map[string]any
	if err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/api/scripts/%d", id), nil, body, &sc); err != nil {
		return nil, err
	}
	return ok(ctx, c, id, map[string]any{"version": sc["version"], "subjects": sc["subjects"], "needs_runner": sc["needs_runner"]}), nil
}

func scriptValidate(ctx context.Context, c *Client, graph any) (any, error) {
	if graph == nil {
		return nil, &Err{422, "validate needs graph"}
	}
	var res map[string]any
	if err := c.Do(ctx, http.MethodPost, "/api/scripts/validate", nil, map[string]any{"graph": graph}, &res); err != nil {
		return nil, err
	}
	res["workspace"] = c.Workspace(ctx)
	return res, nil
}

func scriptSamples(ctx context.Context, c *Client) (any, error) {
	var samples []map[string]any
	if err := c.Do(ctx, http.MethodGet, "/api/script-samples", nil, nil, &samples); err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(samples))
	for _, s := range samples {
		items = append(items, map[string]any{
			"id": s["id"], "title": s["title"], "description": s["description"],
			"subject": s["subject"], "needs_runner": s["needs_runner"],
		})
	}
	return map[string]any{"items": items, "total": len(items), "workspace": c.Workspace(ctx)}, nil
}

func scriptSubjectPath(subject string) (string, error) {
	switch subject {
	case "step":
		return "steps", nil
	case "note":
		return "notes", nil
	case "template":
		return "templates", nil
	default:
		return "", &Err{422, "subject: step, note or template"}
	}
}

func scriptBind(ctx context.Context, c *Client, subject string, id, scriptID int64) (any, error) {
	seg, err := scriptSubjectPath(subject)
	if err != nil {
		return nil, err
	}
	if id < 1 || scriptID < 1 {
		return nil, &Err{422, "bind needs id (the " + subject + ") and script_id"}
	}
	if err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/api/%s/%d/script", seg, id), nil, map[string]any{"script_id": scriptID}, nil); err != nil {
		return nil, err
	}
	return ok(ctx, c, id, map[string]any{"subject": subject, "script_id": scriptID, "bound": true}), nil
}

func scriptUnbind(ctx context.Context, c *Client, subject string, id int64) (any, error) {
	seg, err := scriptSubjectPath(subject)
	if err != nil {
		return nil, err
	}
	if id < 1 {
		return nil, &Err{422, "unbind needs id (the " + subject + ")"}
	}
	if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/%s/%d/script", seg, id), nil, nil, nil); err != nil {
		return nil, err
	}
	return ok(ctx, c, id, map[string]any{"subject": subject, "unbound": true}), nil
}
