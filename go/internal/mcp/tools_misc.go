package mcp

import (
	"context"
	"fmt"
	"net/http"
)

func workspaceTools(c *Client) []Tool {
	type none struct{}
	list := Typed("list_workspaces", "Workspaces of this account (names/ids from the API) and the active one. All calls act in the active workspace.",
		func(ctx context.Context, _ none) (any, error) {
			ws, cur := c.Workspaces(ctx)
			return map[string]any{"workspaces": ws, "current": cur}, nil
		})
	type swArgs struct {
		Name string `json:"name" desc:"workspace name or id from list_workspaces"`
	}
	sw := Typed("switch_workspace", "Make another workspace active for all following calls in this session (not remembered after a restart; the default is the first workspace).",
		func(ctx context.Context, a swArgs) (any, error) {
			prev, cur, err := c.Switch(ctx, a.Name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"previous": prev, "current": cur}, nil
		})
	return []Tool{list, sw}
}

func questlineTools(c *Client) []Tool {
	type none struct{}
	list := Typed("list_questlines", "All questlines (projects) as summaries: id, title, category, color, description_len. Details: get_questline.",
		func(ctx context.Context, _ none) (any, error) {
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/questlines", nil, nil, &rows); err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				items = append(items, map[string]any{
					"id": r["id"], "title": r["title"], "category_id": r["category_id"], "category_label": r["category_label"],
					"color": r["color"], "icon": r["icon"], "updated_at": r["updated_at"],
					"description_len": len([]rune(str(r["description"]))),
				})
			}
			return map[string]any{"items": items, "total": len(items), "workspace": c.Workspace(ctx)}, nil
		})

	type getArgs struct {
		ID       int64 `json:"id"`
		MaxChars int   `json:"max_chars,omitempty"`
		Offset   int   `json:"offset,omitempty"`
		Quests   *int  `json:"quests,omitempty" desc:"how many of its quests to list (summaries, newest first); default 20, 0 = none"`
	}
	get := Typed("get_questline", "One questline: fields, description (cut), and a page of its quests as summaries (any status).",
		func(ctx context.Context, a getArgs) (any, error) {
			var ql map[string]any
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/questlines/%d", a.ID), nil, nil, &ql); err != nil {
				return nil, err
			}
			out := map[string]any{}
			for k, v := range ql {
				if k != "description" {
					out[k] = v
				}
			}
			withText(out, "description", str(ql["description"]), a.MaxChars, a.Offset)
			n := 20
			if a.Quests != nil {
				n = *a.Quests
			}
			if n > 0 {
				q := values("summary", "1", "questline_id", itoa(a.ID), "limit", fmt.Sprint(clampLimit(n, 20)))
				page, err := getPage(ctx, c, "/api/quests", q)
				if err != nil {
					return nil, err
				}
				out["quests"] = page
			}
			out["workspace"] = c.Workspace(ctx)
			return out, nil
		})

	type createArgs struct {
		Title       string  `json:"title"`
		Description *string `json:"description,omitempty" desc:"markdown"`
		Category    string  `json:"category,omitempty" desc:"id or name"`
		Color       *string `json:"color,omitempty"`
		Icon        *string `json:"icon,omitempty"`
	}
	body := func(ctx context.Context, title, cat *string, a createArgs) (map[string]any, error) {
		b := map[string]any{}
		if title != nil {
			b["title"] = *title
		}
		if a.Description != nil {
			b["description"] = *a.Description
		}
		if a.Color != nil {
			b["color"] = *a.Color
		}
		if a.Icon != nil {
			b["icon"] = *a.Icon
		}
		if cat != nil {
			id, err := resolveID(ctx, c, "/api/categories", *cat, "slug", "label")
			if err != nil {
				return nil, err
			}
			if id != nil {
				b["category_id"] = *id
			}
		}
		return b, nil
	}
	create := Typed("create_questline", "Create a questline (a project/series quests attach to).",
		func(ctx context.Context, a createArgs) (any, error) {
			var cat *string
			if a.Category != "" {
				cat = &a.Category
			}
			b, err := body(ctx, &a.Title, cat, a)
			if err != nil {
				return nil, err
			}
			var ql struct {
				ID int64 `json:"id"`
			}
			if err := c.Do(ctx, http.MethodPost, "/api/questlines", nil, b, &ql); err != nil {
				return nil, err
			}
			return ok(ctx, c, ql.ID, nil), nil
		})
	type updArgs struct {
		ID    int64   `json:"id"`
		Title *string `json:"title,omitempty"`
		createArgsNoTitle
	}
	upd := Typed("update_questline", "Change a questline's title, description, category, color or icon.",
		func(ctx context.Context, a updArgs) (any, error) {
			var cat *string
			if a.Category != "" {
				cat = &a.Category
			}
			b, err := body(ctx, a.Title, cat, createArgs{Description: a.Description, Color: a.Color, Icon: a.Icon})
			if err != nil {
				return nil, err
			}
			if len(b) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/questlines/%d", a.ID), nil, b, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, nil), nil
		})
	type delArgs struct {
		ID int64 `json:"id"`
	}
	del := Typed("delete_questline", "Delete a questline; its quests stay (detach first with update_quest clear_questline if the API refuses).",
		func(ctx context.Context, a delArgs) (any, error) {
			if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/questlines/%d", a.ID), nil, nil, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"deleted": true}), nil
		})
	return []Tool{list, get, create, upd, del}
}

type createArgsNoTitle struct {
	Description *string `json:"description,omitempty" desc:"markdown"`
	Category    string  `json:"category,omitempty" desc:"id or name"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}
