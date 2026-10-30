package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func tagTools(c *Client) []Tool {
	type listArgs struct {
		Q string `json:"q,omitempty" desc:"substring of slug/label"`
	}
	list := Typed("list_tags", "Tags (short work-surface labels: front, api, infra…). Look here before create_tag; do not invent near-duplicates.",
		func(ctx context.Context, a listArgs) (any, error) {
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/tags", values("q", a.Q), nil, &rows); err != nil {
				return nil, err
			}
			return map[string]any{"items": rows, "total": len(rows), "workspace": c.Workspace(ctx)}, nil
		})
	type createArgs struct {
		Slug  string `json:"slug,omitempty"`
		Label string `json:"label,omitempty" desc:"≤6 chars is best (sidebar bookmark)"`
		Color string `json:"color,omitempty"`
	}
	create := Typed("create_tag", "Create a tag (slug or label required); an existing one with the same slug is returned instead.",
		func(ctx context.Context, a createArgs) (any, error) {
			if a.Slug == "" && a.Label == "" {
				return nil, &Err{422, "slug or label required"}
			}
			body := map[string]any{}
			for k, v := range map[string]string{"slug": a.Slug, "label": a.Label, "color": a.Color} {
				if v != "" {
					body[k] = v
				}
			}
			var tag map[string]any
			err := c.Do(ctx, http.MethodPost, "/api/tags", nil, body, &tag)
			if e, isErr := err.(*Err); isErr && e.Code == http.StatusConflict {
				// the API answers 409 with the existing tag in the message: re-read by slug
				var rows []map[string]any
				key := a.Slug
				if key == "" {
					key = a.Label
				}
				if rerr := c.Do(ctx, http.MethodGet, "/api/tags", values("q", key), nil, &rows); rerr == nil && len(rows) > 0 {
					return map[string]any{"tag": rows[0], "existing": true, "workspace": c.Workspace(ctx)}, nil
				}
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"tag": tag, "workspace": c.Workspace(ctx)}, nil
		})
	type setArgs struct {
		Owner  string   `json:"owner" desc:"quest=N or template=N"`
		TagIDs []int64  `json:"tag_ids,omitempty"`
		Tags   []string `json:"tags,omitempty" desc:"slugs/labels, alternative to tag_ids"`
	}
	set := Typed("set_tags", "Replace the tags of a quest or template (max 5; none given = clear).",
		func(ctx context.Context, a setArgs) (any, error) {
			kind, id, err := parseOwner(a.Owner)
			if err != nil {
				return nil, err
			}
			seg := map[string]string{"quest": "quests", "template": "templates"}[kind]
			if seg == "" {
				return nil, &Err{422, "owner must be quest=N or template=N"}
			}
			body := map[string]any{}
			if a.TagIDs != nil {
				body["tag_ids"] = a.TagIDs
			}
			if a.Tags != nil {
				body["tags"] = a.Tags
			}
			if len(body) == 0 {
				body["tag_ids"] = []int64{}
			}
			var tags []map[string]any
			if err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/api/%s/%d/tags", seg, id), nil, body, &tags); err != nil {
				return nil, err
			}
			return ok(ctx, c, id, map[string]any{"tags": tags}), nil
		})
	return []Tool{list, create, set}
}

var secretOwners = map[string]string{"questline": "questlines", "template": "templates", "quest": "quests", "step": "steps", "note": "notes"}

// parseOwner reads "quest=12" into its kind and id.
func parseOwner(s string) (string, int64, error) {
	kind, raw, _ := strings.Cut(strings.TrimSpace(s), "=")
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || kind == "" {
		return "", 0, &Err{422, fmt.Sprintf("owner must look like 'quest=12' (got %q)", s)}
	}
	return strings.ToLower(strings.TrimSpace(kind)), id, nil
}

func secretPath(owner string) (string, error) { return ownerPath(owner, "secrets") }

// ownerPath is the API path of an owner's collection (`secrets`, `values`).
func ownerPath(owner, collection string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(owner), "workspace") {
		return "/api/workspace/" + collection, nil // the workspace has no id: it is the active one
	}
	kind, id, err := parseOwner(owner)
	if err != nil {
		return "", err
	}
	seg, ok := secretOwners[kind]
	if !ok {
		return "", &Err{422, "owner must be one of: questline=N, template=N, quest=N, step=N, note=N, workspace"}
	}
	return fmt.Sprintf("/api/%s/%d/%s", seg, id, collection), nil
}

func secretTools(c *Client) []Tool {
	type listArgs struct {
		Owner string `json:"owner" desc:"questline=N | template=N | quest=N | step=N | note=N | workspace (the active one: inherited by everything in it)"`
	}
	list := Typed("list_secrets", "Secrets of an owner: own, inherited, effective name/source rows (same groups as list_values), plus legacy keys. Notes inherit parents and workspace; private notes are hidden from MCP. Secret values are never readable.",
		func(ctx context.Context, a listArgs) (any, error) {
			p, err := secretPath(a.Owner)
			if err != nil {
				return nil, err
			}
			var res map[string]any
			if err := c.Do(ctx, http.MethodGet, p, nil, nil, &res); err != nil {
				return nil, err
			}
			res["workspace"] = c.Workspace(ctx)
			return res, nil
		})
	type setArgs struct {
		Owner string `json:"owner"`
		Key   string `json:"key" desc:"env var name"`
		Value string `json:"value" desc:"write-only"`
	}
	set := Typed("set_secret", "Set a secret (an env var for the checks/commands of the owner and everything under it). Put it on the narrowest owner that needs it.",
		func(ctx context.Context, a setArgs) (any, error) {
			p, err := secretPath(a.Owner)
			if err != nil {
				return nil, err
			}
			if err := c.Do(ctx, http.MethodPut, p+"/"+url.PathEscape(a.Key), nil, map[string]any{"value": a.Value}, nil); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "owner": a.Owner, "key": a.Key, "workspace": c.Workspace(ctx)}, nil
		})
	type delArgs struct {
		Owner string `json:"owner"`
		Key   string `json:"key"`
	}
	del := Typed("delete_secret", "Remove one secret from an owner.",
		func(ctx context.Context, a delArgs) (any, error) {
			p, err := secretPath(a.Owner)
			if err != nil {
				return nil, err
			}
			if err := c.Do(ctx, http.MethodDelete, p+"/"+url.PathEscape(a.Key), nil, nil, nil); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "owner": a.Owner, "key": a.Key, "deleted": true, "workspace": c.Workspace(ctx)}, nil
		})
	return []Tool{list, set, del}
}

func valueTools(c *Client) []Tool {
	type listArgs struct {
		Owner string `json:"owner" desc:"questline=N | template=N | quest=N | step=N | note=N | workspace (the active one)"`
	}
	list := Typed("list_values", "Variables («переменные» in the interface; `values` in the API) of an owner: own, inherited from parents and the workspace, and the effective set with formulas already computed. Variables are readable data (unlike secrets); a private note answers 403.",
		func(ctx context.Context, a listArgs) (any, error) {
			p, err := ownerPath(a.Owner, "values")
			if err != nil {
				return nil, err
			}
			var res map[string]any
			if err := c.Do(ctx, http.MethodGet, p, nil, nil, &res); err != nil {
				return nil, err
			}
			res["workspace"] = c.Workspace(ctx)
			return res, nil
		})
	type setArgs struct {
		Owner   string `json:"owner"`
		Name    string `json:"name" desc:"lower-case latin, digits, _, up to 24 chars, starts with a letter"`
		Type    string `json:"type" desc:"int | float | str | bool"`
		Value   any    `json:"value,omitempty" desc:"the literal: a number, a string or true/false"`
		Formula string `json:"formula,omitempty" desc:"instead of value: an expression over other variables, e.g. (str - 10) / 2; integer / and % round down; strings join with +"`
	}
	set := Typed("set_value", "Set a variable («переменная» in the interface): a literal (value) or a formula over other variables in scope. A note inherits the variables of its parents and the workspace; text shows one as {{name}}, a formula or script=N{...} reads it by name. Put it on the narrowest owner.",
		func(ctx context.Context, a setArgs) (any, error) {
			p, err := ownerPath(a.Owner, "values")
			if err != nil {
				return nil, err
			}
			body := map[string]any{"type": a.Type}
			if strings.TrimSpace(a.Formula) != "" {
				body["formula"] = a.Formula
			} else {
				body["value"] = a.Value
			}
			var res map[string]any
			if err := c.Do(ctx, http.MethodPut, p+"/"+url.PathEscape(a.Name), nil, body, &res); err != nil {
				return nil, err
			}
			res["owner"] = a.Owner
			res["workspace"] = c.Workspace(ctx)
			return res, nil
		})
	type delArgs struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}
	del := Typed("delete_value", "Remove one variable from an owner.",
		func(ctx context.Context, a delArgs) (any, error) {
			p, err := ownerPath(a.Owner, "values")
			if err != nil {
				return nil, err
			}
			if err := c.Do(ctx, http.MethodDelete, p+"/"+url.PathEscape(a.Name), nil, nil, nil); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "owner": a.Owner, "name": a.Name, "deleted": true, "workspace": c.Workspace(ctx)}, nil
		})
	return []Tool{list, set, del}
}

func runnerTools(c *Client) []Tool {
	type none struct{}
	runners := Typed("list_runners", "Runners (devices that execute step checks and template commands): label, online, last seen.",
		func(ctx context.Context, _ none) (any, error) {
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/runners", nil, nil, &rows); err != nil {
				return nil, err
			}
			return map[string]any{"items": rows, "total": len(rows), "workspace": c.Workspace(ctx)}, nil
		})
	type jobsArgs struct {
		Label string `json:"label,omitempty"`
		State string `json:"state,omitempty"`
		Limit int    `json:"limit,omitempty" desc:"default 10, max 100"`
	}
	jobs := Typed("list_runner_jobs", "Recent runner jobs (state, exit code, progress); stdout/stderr tails cut to 1500 chars.",
		func(ctx context.Context, a jobsArgs) (any, error) {
			lim := clampLimit(a.Limit, 10)
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/runner-jobs", values("label", a.Label, "state", a.State, "limit", strconv.Itoa(lim)), nil, &rows); err != nil {
				return nil, err
			}
			keep := []string{"id", "state", "kind", "runner_label", "quest_id", "step_id", "template_id", "title", "attempt_no", "created_at",
				"assigned_at", "started_at", "finished_at", "valid_until", "exit_code", "timed_out", "progress_current", "progress_total", "reason"}
			items := make([]map[string]any, 0, len(rows))
			for _, j := range rows {
				it := map[string]any{}
				for _, k := range keep {
					if v := j[k]; v != nil && v != "" {
						it[k] = v
					}
				}
				for _, k := range []string{"stdout", "stderr", "log_tail"} {
					if s := str(j[k]); s != "" {
						it[k] = tail(s, 1500)
					}
				}
				items = append(items, it)
			}
			return map[string]any{"items": items, "total": len(items), "truncated": len(rows) >= lim, "workspace": c.Workspace(ctx)}, nil
		})
	type cancelArgs struct {
		ID int64 `json:"id" desc:"job id"`
	}
	cancel := Typed("cancel_runner_job", "Cancel a queued or running runner job.",
		func(ctx context.Context, a cancelArgs) (any, error) {
			if err := c.Do(ctx, http.MethodPost, fmt.Sprintf("/api/runner-jobs/%d/cancel", a.ID), nil, nil, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"cancelled": true}), nil
		})
	return []Tool{runners, jobs, cancel}
}

func tail(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return "…" + string(r[len(r)-n:])
	}
	return s
}

func graphTools(c *Client) []Tool {
	type args struct {
		Entity   string `json:"entity" enum:"quest,note,questline"`
		ID       int64  `json:"id"`
		Depth    int    `json:"depth,omitempty" desc:"1..4, default 2"`
		MaxNodes int    `json:"max_nodes,omitempty" desc:"default 60"`
	}
	g := Typed("get_graph", "Links around a quest/note/questline (who cites whom) as nodes (key,type,id,title,status,depth) and edges.",
		func(ctx context.Context, a args) (any, error) {
			depth := min(4, max(1, a.Depth))
			if a.Depth == 0 {
				depth = 2
			}
			var res struct {
				Root      any              `json:"root"`
				Depth     any              `json:"depth"`
				Truncated bool             `json:"truncated"`
				Nodes     []map[string]any `json:"nodes"`
				Edges     []map[string]any `json:"edges"`
			}
			if err := c.Do(ctx, http.MethodGet, "/api/graph", values(a.Entity, itoa(a.ID), "depth", strconv.Itoa(depth)), nil, &res); err != nil {
				return nil, err
			}
			limit := a.MaxNodes
			if limit <= 0 {
				limit = 60
			}
			trunc := res.Truncated
			nodes := make([]map[string]any, 0, min(limit, len(res.Nodes)))
			keep := map[string]bool{}
			for _, n := range res.Nodes {
				if len(nodes) == limit {
					trunc = true
					break
				}
				row := map[string]any{}
				for _, k := range []string{"key", "type", "id", "title", "status", "depth"} {
					if v := n[k]; v != nil && v != "" {
						row[k] = v
					}
				}
				keep[str(n["key"])] = true
				nodes = append(nodes, row)
			}
			edges := make([]map[string]any, 0, len(res.Edges))
			for _, e := range res.Edges {
				if keep[str(e["from"])] && keep[str(e["to"])] {
					edges = append(edges, map[string]any{"from": e["from"], "to": e["to"], "kind": e["kind"]})
				}
			}
			return map[string]any{"root": res.Root, "depth": res.Depth, "truncated": trunc, "nodes": nodes, "edges": edges, "workspace": c.Workspace(ctx)}, nil
		})
	return []Tool{g, graphLayoutTool(c)}
}
