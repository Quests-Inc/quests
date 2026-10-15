package mcp

import (
	"context"
	"fmt"
	"net/http"
)

type stepPatch struct {
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty" desc:"markdown"`
	ProgressCurrent *int    `json:"progress_current,omitempty"`
	ProgressTotal   *int    `json:"progress_total,omitempty"`
	stepOpts
}

// stepOpts are the optional check/ordering settings shared by add and update.
type stepOpts struct {
	SortOrder            *int    `json:"sort_order,omitempty"`
	CheckCommand         *string `json:"check_command,omitempty" desc:"shell or #!script run on a runner; needs run_on"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds,omitempty"`
	WaitPrevious         *bool   `json:"wait_previous,omitempty"`
	RunMode              *string `json:"run_mode,omitempty" enum:"poll,once,watch"`
	RunOn                *string `json:"run_on,omitempty" desc:"runner label, or a,b = first online"`
}

func (p stepPatch) body() map[string]any {
	b := map[string]any{}
	if p.Title != nil {
		b["title"] = *p.Title
	}
	if p.Description != nil {
		b["description"] = *p.Description
	}
	if p.ProgressCurrent != nil {
		b["progress_current"] = *p.ProgressCurrent
	}
	if p.ProgressTotal != nil {
		b["progress_total"] = max(1, *p.ProgressTotal)
	}
	stepIn{SortOrder: p.SortOrder, CheckCommand: p.CheckCommand, CheckIntervalSeconds: p.CheckIntervalSeconds,
		WaitPrevious: p.WaitPrevious, RunMode: p.RunMode, RunOn: p.RunOn}.patchInto(b)
	return b
}

func stepTools(c *Client) []Tool {
	type addArgs struct {
		QuestID int64 `json:"quest_id"`
		stepIn
		Quiet *bool `json:"quiet,omitempty"`
	}
	add := Typed("add_step", "Append a step to a quest. Returns the new step's id as last_step_id.",
		func(ctx context.Context, a addArgs) (any, error) {
			var q questMutation
			path := fmt.Sprintf("/api/quests/%d/steps", a.QuestID)
			if err := c.Do(ctx, http.MethodPost, path, mutationQuery(a.Quiet), a.body(), &q); err != nil {
				return nil, err
			}
			return q.result(ctx, c, true), nil
		})

	type updArgs struct {
		QuestID int64 `json:"quest_id"`
		StepID  int64 `json:"step_id"`
		stepPatch
		Quiet *bool `json:"quiet,omitempty"`
	}
	upd := Typed("update_step", "Change a step; progress_current>=progress_total closes it. Several steps at once: update_steps.",
		func(ctx context.Context, a updArgs) (any, error) {
			body := a.body()
			if len(body) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			var q questMutation
			path := fmt.Sprintf("/api/quests/%d/steps/%d", a.QuestID, a.StepID)
			if err := c.Do(ctx, http.MethodPatch, path, mutationQuery(a.Quiet), body, &q); err != nil {
				return nil, err
			}
			return q.result(ctx, c, false), nil
		})

	type delArgs struct {
		QuestID int64 `json:"quest_id"`
		StepID  int64 `json:"step_id"`
		Quiet   *bool `json:"quiet,omitempty"`
	}
	del := Typed("delete_step", "Delete a step (the last step of a quest cannot be deleted).",
		func(ctx context.Context, a delArgs) (any, error) {
			var q questMutation
			path := fmt.Sprintf("/api/quests/%d/steps/%d", a.QuestID, a.StepID)
			if err := c.Do(ctx, http.MethodDelete, path, mutationQuery(a.Quiet), nil, &q); err != nil {
				return nil, err
			}
			return q.result(ctx, c, false), nil
		})

	type op struct {
		Op     string `json:"op" enum:"add,update,delete"`
		StepID int64  `json:"step_id,omitempty" desc:"required for update and delete"`
		stepPatch
	}
	type batchArgs struct {
		QuestID int64 `json:"quest_id"`
		Ops     []op  `json:"ops" desc:"applied in order; a failed op does not stop the rest"`
		Quiet   *bool `json:"quiet,omitempty"`
	}
	batch := Typed("update_steps", "Several step changes on one quest in one call: ops of add (title required) / update / delete. Returns a result per op and the quest's final progress_label.",
		func(ctx context.Context, a batchArgs) (any, error) {
			if len(a.Ops) == 0 {
				return nil, &Err{422, "ops is empty"}
			}
			results := make([]map[string]any, 0, len(a.Ops))
			label := ""
			for i, o := range a.Ops {
				r := map[string]any{"i": i, "op": o.Op}
				var q questMutation
				var err error
				base := fmt.Sprintf("/api/quests/%d/steps", a.QuestID)
				switch o.Op {
				case "add":
					if o.Title == nil || *o.Title == "" {
						err = &Err{422, "add needs title"}
						break
					}
					in := stepIn{Title: *o.Title, Description: o.Description, ProgressTotal: o.ProgressTotal,
						ProgressCurrent: o.ProgressCurrent, SortOrder: o.SortOrder, CheckCommand: o.CheckCommand,
						CheckIntervalSeconds: o.CheckIntervalSeconds, WaitPrevious: o.WaitPrevious, RunMode: o.RunMode, RunOn: o.RunOn}
					if err = c.Do(ctx, http.MethodPost, base, mutationQuery(a.Quiet), in.body(), &q); err == nil && len(q.Steps) > 0 {
						r["step_id"] = q.Steps[len(q.Steps)-1].ID
					}
				case "update":
					body := o.body()
					switch {
					case o.StepID == 0:
						err = &Err{422, "update needs step_id"}
					case len(body) == 0:
						err = &Err{422, "update needs at least one field"}
					default:
						err = c.Do(ctx, http.MethodPatch, fmt.Sprintf("%s/%d", base, o.StepID), mutationQuery(a.Quiet), body, &q)
						r["step_id"] = o.StepID
					}
				case "delete":
					if o.StepID == 0 {
						err = &Err{422, "delete needs step_id"}
						break
					}
					err = c.Do(ctx, http.MethodDelete, fmt.Sprintf("%s/%d", base, o.StepID), mutationQuery(a.Quiet), nil, &q)
					r["step_id"] = o.StepID
				default:
					err = &Err{422, fmt.Sprintf("unknown op %q", o.Op)}
				}
				if err != nil {
					r["ok"], r["error"] = false, err.Error()
				} else {
					r["ok"] = true
					label = q.ProgressLabel
				}
				results = append(results, r)
			}
			return ok(ctx, c, a.QuestID, map[string]any{"results": results, "progress_label": label}), nil
		})

	return []Tool{add, upd, del, batch}
}
