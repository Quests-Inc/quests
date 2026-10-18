package mcp

import (
	"context"
	"fmt"
	"net/http"
)

// hookTools is one multi-action tool for workspace event hooks (quest=438): an event trigger plus a library script that a
// runner (or the server's own HTTP/socket nodes) carries out. Owner-only to change.
func hookTools(c *Client) []Tool {
	type args struct {
		Action     string   `json:"action" enum:"list,get,create,update,delete,events"`
		ID         int64    `json:"id,omitempty" desc:"hook id (get/update/delete)"`
		Name       *string  `json:"name,omitempty" desc:"create/update"`
		Enabled    *bool    `json:"enabled,omitempty" desc:"create (default true) / update"`
		Events     []string `json:"events,omitempty" desc:"create (required) / update: aliases or kinds from action=events, e.g. complete, step, fail, quest_started"`
		ScriptID   int64    `json:"script_id,omitempty" desc:"create (required) / update: a library script (see the script tool)"`
		QuestID    *int64   `json:"quest_id,omitempty" desc:"create/update: limit the hook to one quest; 0 on update detaches it (whole journal)"`
		RunOn      *string  `json:"run_on,omitempty" desc:"create/update: runner label for the script's code, or '' when it has none"`
		TimeoutSec *int     `json:"timeout_sec,omitempty" desc:"create/update: 1..600, default 30"`
		Limit      int      `json:"limit,omitempty" desc:"list: default 30, max 100"`
	}
	s := Typed("hook", "Workspace event hooks: an event (quest completed, step progress, failed, …) runs a library script. Actions: list, get, create (events + script_id), update (any field; quest_id=0 detaches from the quest), delete, events (the aliases and kinds you can use). Owner-only for create/update/delete.",
		func(ctx context.Context, a args) (any, error) {
			return hookAction(ctx, c, "hook_"+a.Action, hookArgs{ID: a.ID, Name: a.Name, Enabled: a.Enabled, Events: a.Events, ScriptID: a.ScriptID,
				QuestID: a.QuestID, RunOn: a.RunOn, TimeoutSec: a.TimeoutSec, Limit: a.Limit})
		})
	return []Tool{s}
}

type hookArgs struct {
	ID         int64
	Name       *string
	Enabled    *bool
	Events     []string
	ScriptID   int64
	QuestID    *int64
	RunOn      *string
	TimeoutSec *int
	Limit      int
}

func hookAction(ctx context.Context, c *Client, action string, a hookArgs) (any, error) {
	switch action {
	case "hook_list":
		var rows []map[string]any
		if err := c.Do(ctx, http.MethodGet, "/api/hooks", nil, nil, &rows); err != nil {
			return nil, err
		}
		limit := a.Limit
		if limit < 1 {
			limit = 30
		}
		if limit > 100 {
			limit = 100
		}
		total := len(rows)
		if len(rows) > limit {
			rows = rows[:limit]
		}
		return map[string]any{"items": rows, "total": total, "truncated": total > len(rows), "workspace": c.Workspace(ctx)}, nil
	case "hook_get":
		if a.ID < 1 {
			return nil, &Err{422, "get needs id"}
		}
		var h map[string]any
		if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/hooks/%d", a.ID), nil, nil, &h); err != nil {
			return nil, err
		}
		return h, nil
	case "hook_events":
		var cat map[string]any
		if err := c.Do(ctx, http.MethodGet, "/api/hook-events", nil, nil, &cat); err != nil {
			return nil, err
		}
		return cat, nil
	case "hook_create":
		if len(a.Events) == 0 || a.ScriptID < 1 {
			return nil, &Err{422, "create needs events and script_id"}
		}
		var h map[string]any
		if err := c.Do(ctx, http.MethodPost, "/api/hooks", nil, hookBody(a.Name, a.Enabled, a.Events, a.ScriptID, a.QuestID, a.RunOn, a.TimeoutSec), &h); err != nil {
			return nil, err
		}
		return h, nil
	case "hook_update":
		if a.ID < 1 {
			return nil, &Err{422, "update needs id"}
		}
		body := hookBody(a.Name, a.Enabled, a.Events, a.ScriptID, a.QuestID, a.RunOn, a.TimeoutSec)
		if len(body) == 0 {
			return nil, &Err{422, "provide at least one field to change"}
		}
		var h map[string]any
		if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/hooks/%d", a.ID), nil, body, &h); err != nil {
			return nil, err
		}
		return h, nil
	case "hook_delete":
		if a.ID < 1 {
			return nil, &Err{422, "delete needs id"}
		}
		if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/hooks/%d", a.ID), nil, nil, nil); err != nil {
			return nil, err
		}
		return ok(ctx, c, a.ID, map[string]any{"deleted": true}), nil
	}
	return nil, &Err{422, "action: list, get, create, update, delete or events"}
}

// hookBody is the request body from the fields that were given; quest_id 0 is sent as null (detach).
func hookBody(name *string, enabled *bool, events []string, scriptID int64, questID *int64, runOn *string, timeout *int) map[string]any {
	body := map[string]any{}
	if name != nil {
		body["name"] = *name
	}
	if enabled != nil {
		body["enabled"] = *enabled
	}
	if len(events) > 0 {
		body["events"] = events
	}
	if scriptID > 0 {
		body["script_id"] = scriptID
	}
	if questID != nil {
		if *questID == 0 {
			body["quest_id"] = nil
		} else {
			body["quest_id"] = *questID
		}
	}
	if runOn != nil {
		body["run_on"] = *runOn
	}
	if timeout != nil {
		body["timeout_sec"] = *timeout
	}
	return body
}
