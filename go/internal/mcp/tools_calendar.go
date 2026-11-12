package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/quests-inc/quests/go/internal/platform/search"
)

type calendarFields struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty" desc:"markdown"`
	StartsAt    *string `json:"starts_at,omitempty" desc:"RFC3339 with timezone, converted to UTC"`
	EndsAt      *string `json:"ends_at,omitempty" desc:"RFC3339, not before starts_at"`
	AllDay      *bool   `json:"all_day,omitempty"`
	Color       *string `json:"color,omitempty" desc:"hex colour"`
	Icon        *string `json:"icon,omitempty" desc:"built-in icon name; custom images use icon(event=N)"`
}

func calendarTools(c *Client) []Tool {
	type args struct {
		Action string `json:"action" enum:"list,get,create,update,delete"`
		ID     int64  `json:"id,omitempty" desc:"get/update/delete"`
		calendarFields
		Query    string `json:"q,omitempty" desc:"list: shared text/ID search, e.g. event=12 or 12"`
		Limit    int    `json:"limit,omitempty" desc:"list: default 20, max 100"`
		Cursor   string `json:"cursor,omitempty" desc:"list: next from the previous page"`
		MaxChars int    `json:"max_chars,omitempty" desc:"get: description cut, default 4000"`
		Offset   int    `json:"offset,omitempty" desc:"get: rune offset to continue description"`
	}
	event := Typed("event", "Calendar events (start/end/all-day), distinct from automation hook signals. Actions: list (compact pages; q uses shared search), get, create (title, starts_at, ends_at), update (only supplied fields), delete (permanently removes the event; linked quests keep their resolved date as a fixed deadline, urgency window and expiry policy). Quest bindings are live: moving an event recomputes their due dates atomically. Use create_quest/update_quest event_id, event_edge, event_dir and event_offset_seconds; duration_seconds and expire_on_deadline work with either deadline source. Custom images: icon(event=N).", func(ctx context.Context, a args) (any, error) {
		if a.Action != "list" && a.Action != "create" && a.ID <= 0 {
			return nil, &Err{422, "provide a positive event id"}
		}
		path := fmt.Sprintf("/api/calendar/events/%d", a.ID)
		switch a.Action {
		case "list":
			offset := 0
			if a.Cursor != "" {
				var err error
				offset, err = strconv.Atoi(a.Cursor)
				if err != nil || offset < 0 {
					return nil, &Err{422, "invalid event cursor"}
				}
			}
			limit := clampLimit(a.Limit, 20)
			if a.Query != "" {
				var page search.Page
				if err := c.Do(ctx, http.MethodPost, "/api/search", nil, search.Request{Query: a.Query, Types: []string{"event"}, Limit: limit, Offset: offset}, &page); err != nil {
					return nil, err
				}
				items := []map[string]any{}
				for _, hit := range page.Hits {
					var row map[string]any
					if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/calendar/events/%d", hit.ID), nil, nil, &row); err != nil {
						return nil, err
					}
					items = append(items, calendarSummary(row))
				}
				next := ""
				if page.More {
					next = strconv.Itoa(page.Next)
				}
				return map[string]any{"items": items, "truncated": page.More, "next": next, "workspace": c.Workspace(ctx)}, nil
			}
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/calendar/events", nil, nil, &rows); err != nil {
				return nil, err
			}
			start := min(offset, len(rows))
			end := min(start+limit, len(rows))
			items := []map[string]any{}
			for _, row := range rows[start:end] {
				items = append(items, calendarSummary(row))
			}
			next := ""
			if end < len(rows) {
				next = strconv.Itoa(end)
			}
			return map[string]any{"items": items, "total": len(rows), "truncated": end < len(rows), "next": next, "workspace": c.Workspace(ctx)}, nil
		case "get":
			var row map[string]any
			if err := c.Do(ctx, http.MethodGet, path, nil, nil, &row); err != nil {
				return nil, err
			}
			withText(row, "description", str(row["description"]), a.MaxChars, a.Offset)
			row["workspace"] = c.Workspace(ctx)
			return row, nil
		case "create", "update":
			raw, _ := json.Marshal(a.calendarFields)
			body := map[string]any{}
			_ = json.Unmarshal(raw, &body)
			if a.Action == "create" && (a.Title == nil || a.StartsAt == nil || a.EndsAt == nil) {
				return nil, &Err{422, "create needs title, starts_at and ends_at"}
			}
			if len(body) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			method := http.MethodPatch
			if a.Action == "create" {
				method = http.MethodPost
				path = "/api/calendar/events"
			}
			var row map[string]any
			if err := c.Do(ctx, method, path, nil, body, &row); err != nil {
				return nil, err
			}
			return ok(ctx, c, int64(num(row["id"])), map[string]any{"title": row["title"], "starts_at": row["starts_at"], "ends_at": row["ends_at"]}), nil
		case "delete":
			if err := c.Do(ctx, http.MethodDelete, path, nil, nil, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"linked_quests": "kept their last resolved fixed deadline and options"}), nil
		default:
			return nil, &Err{422, "unknown calendar event action"}
		}
	})
	find := Typed("search_entities", "Shared workspace entity search: exact kind=ID or bare ID first, then title/body and title typos. Types narrow the entity kinds; filters and allowed/excluded scopes apply before pagination. Returns hits {kind,id,title,where,snippet,parent_kind,parent_id}, more and next (offset). Private notes are excluded for MCP. This is entity discovery, not in-document text search.", func(ctx context.Context, a search.Request) (any, error) {
		var page map[string]any
		if err := c.Do(ctx, http.MethodPost, "/api/search", nil, a, &page); err != nil {
			return nil, err
		}
		page["workspace"] = c.Workspace(ctx)
		return page, nil
	})
	return []Tool{event, find}
}
func calendarSummary(row map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "title", "starts_at", "ends_at", "all_day", "color", "icon", "icon_url", "updated_at"} {
		out[key] = row[key]
	}
	out["description_len"] = len([]rune(str(row["description"])))
	return out
}
