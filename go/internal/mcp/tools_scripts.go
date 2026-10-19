package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// scriptTools is one multi-action tool (like trash): the agent works with the script library
// without one tool per action.
func scriptTools(c *Client) []Tool {
	type args struct {
		Action      string            `json:"action" enum:"list,get,create,update,delete,validate,nodes,samples,bind,unbind,custom_list,custom_get,custom_create,custom_update,custom_delete,custom_starters,eval"`
		ID          int64             `json:"id,omitempty" desc:"script id (get/update/delete), the step/note/template id (bind/unbind)"`
		Version     int               `json:"version,omitempty" desc:"get: a past graph version; update: the version you read (always required)"`
		Title       string            `json:"title,omitempty" desc:"create (required) / update"`
		Description *string           `json:"description,omitempty" desc:"create/update"`
		Purpose     *string           `json:"purpose,omitempty" desc:"create/update: what the script is for (step, note, template, hook, embed = a script embedded in a text as script=N{...}; empty = unsorted)"`
		Graph       any               `json:"graph,omitempty" desc:"create/update/validate: node graph (docs/adr/0003-script-graphs.md, docs/automation.md)"`
		Node        any               `json:"node,omitempty" desc:"custom_create/custom_update: {title, description?, inputs:[{name,node,port}], outputs:[{name,node,port}], fields:[{name,label?,type:str|int|bool,default?,node,param}], graph}; update also takes version; param is a key of the inner node's params or vars.NAME of a code node; used in a graph as type custom.<id>"`
		Sample      string            `json:"sample,omitempty" desc:"create: copy a sample id from action=samples instead of graph"`
		Subject     string            `json:"subject,omitempty" enum:"step,note,template" desc:"bind/unbind: what to attach the script to"`
		ScriptID    int64             `json:"script_id,omitempty" desc:"bind / hook_create / hook_update: which script"`
		Limit       int               `json:"limit,omitempty" desc:"list: default 20, max 100"`
		Args        map[string]string `json:"args,omitempty" desc:"eval: named arguments of the script's Args node, as text"`
	}
	s := Typed("script", "Workspace scripts (ADR-0003: graphs of typed nodes). Actions: list (summaries), get (with graph; version for a past save), create (graph or sample id from samples), update (pass version you read), delete, validate (check a graph without saving), nodes (catalog), samples (starters), bind / unbind (subject=step|note|template), eval (id + args: run a script made of server nodes only, Args → … → Result, and get its value: the same as an embedded `script=N`). Custom nodes (baked from nodes or one code node, with editable fields) are custom_* actions; custom_starters installs the starting library (the old samples) as custom nodes. Event hooks that run a script are the `hook` tool. Format: docs/adr/0003-script-graphs.md and docs/automation.md. Owner-only for create/update/delete/bind.",
		func(ctx context.Context, a args) (any, error) {
			switch a.Action {
			case "list":
				return scriptList(ctx, c, a.Limit)
			case "get":
				return scriptGet(ctx, c, a.ID, a.Version)
			case "create":
				return scriptCreate(ctx, c, a.Title, a.Description, a.Purpose, a.Graph, a.Sample)
			case "update":
				return scriptUpdate(ctx, c, a.ID, a.Version, a.Title, a.Description, a.Purpose, a.Graph)
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
			case "eval":
				if a.ID < 1 {
					return nil, &Err{422, "eval needs id (and args)"}
				}
				var out map[string]any
				if err := c.Do(ctx, http.MethodPost, fmt.Sprintf("/api/scripts/%d/eval", a.ID), nil, map[string]any{"args": a.Args}, &out); err != nil {
					return nil, err
				}
				return map[string]any{"value": out["value"], "version": out["version"], "workspace": c.Workspace(ctx)}, nil
			case "samples":
				return scriptSamples(ctx, c)
			case "bind":
				return scriptBind(ctx, c, a.Subject, a.ID, a.ScriptID)
			case "unbind":
				return scriptUnbind(ctx, c, a.Subject, a.ID)
			case "custom_list", "custom_get", "custom_create", "custom_update", "custom_delete", "custom_starters":
				return scriptCustom(ctx, c, a.Action, a.ID, a.Node)
			default:
				return nil, &Err{422, "action: list, get, create, update, delete, validate, nodes, samples, bind, unbind or eval, or custom_list / custom_get / custom_create / custom_update / custom_delete"}
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
			"subjects": r["subjects"], "purpose": r["purpose"], "needs_runner": r["needs_runner"], "template": r["template"],
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

func scriptCreate(ctx context.Context, c *Client, title string, description *string, purpose *string, graph any, sample string) (any, error) {
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
	if purpose != nil {
		body["purpose"] = *purpose
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

func scriptUpdate(ctx context.Context, c *Client, id int64, version int, title string, description *string, purpose *string, graph any) (any, error) {
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
	if purpose != nil {
		body["purpose"] = *purpose
		changed = true
	}
	if graph != nil {
		body["graph"] = graph
		changed = true
	}
	if !changed {
		return nil, &Err{422, "provide title, description, purpose and/or graph to update"}
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
			"id": s["id"], "title": s["title"], "description": s["description"], "purpose": s["purpose"],
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

// scriptCustom is the custom-node actions of the script tool: the definition goes through as the API takes it.
func scriptCustom(ctx context.Context, c *Client, action string, id int64, node any) (any, error) {
	const base = "/api/script-custom-nodes"
	switch action {
	case "custom_list":
		var rows []map[string]any
		if err := c.Do(ctx, http.MethodGet, base, nil, nil, &rows); err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]any{
				"id": r["id"], "type": fmt.Sprintf("custom.%v", r["id"]), "title": r["title"], "version": r["version"],
				"needs_runner": r["needs_runner"], "problem": r["problem"],
			})
		}
		return map[string]any{"items": items, "total": len(items), "workspace": c.Workspace(ctx)}, nil
	case "custom_get":
		if id < 1 {
			return nil, &Err{422, "custom_get needs id"}
		}
		var n map[string]any
		if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("%s/%d", base, id), nil, nil, &n); err != nil {
			return nil, err
		}
		n["workspace"] = c.Workspace(ctx)
		return n, nil
	case "custom_create":
		if node == nil {
			return nil, &Err{422, "custom_create needs node"}
		}
		var n map[string]any
		if err := c.Do(ctx, http.MethodPost, base, nil, node, &n); err != nil {
			return nil, err
		}
		nid, _ := n["id"].(float64)
		return ok(ctx, c, int64(nid), map[string]any{"type": fmt.Sprintf("custom.%v", n["id"]), "version": n["version"], "needs_runner": n["needs_runner"]}), nil
	case "custom_update":
		if id < 1 || node == nil {
			return nil, &Err{422, "custom_update needs id and node (with version: the one custom_get returned)"}
		}
		var n map[string]any
		if err := c.Do(ctx, http.MethodPut, fmt.Sprintf("%s/%d", base, id), nil, node, &n); err != nil {
			return nil, err
		}
		return ok(ctx, c, id, map[string]any{"version": n["version"], "needs_runner": n["needs_runner"]}), nil
	case "custom_starters":
		var out map[string]any
		if err := c.Do(ctx, http.MethodPost, base+"/starters", nil, map[string]any{}, &out); err != nil {
			return nil, err
		}
		made, _ := out["created"].([]any)
		return map[string]any{"ok": true, "created": len(made), "workspace": c.Workspace(ctx)}, nil
	default: // custom_delete
		if id < 1 {
			return nil, &Err{422, "custom_delete needs id"}
		}
		if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("%s/%d", base, id), nil, nil, nil); err != nil {
			return nil, err
		}
		return ok(ctx, c, id, map[string]any{"deleted": true}), nil
	}
}
