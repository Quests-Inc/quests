package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

var (
	statusEnum = "active,expired,frozen,completed,failed,archived"
)

type stepIn struct {
	Title                string  `json:"title" desc:"plain text"`
	Description          *string `json:"description,omitempty" desc:"markdown"`
	ProgressTotal        *int    `json:"progress_total,omitempty" desc:"default 1"`
	ProgressCurrent      *int    `json:"progress_current,omitempty" desc:"default 0; current>=total means done"`
	SortOrder            *int    `json:"sort_order,omitempty"`
	CheckCommand         *string `json:"check_command,omitempty" desc:"shell or #!script run on a runner; needs run_on"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds,omitempty"`
	WaitPrevious         *bool   `json:"wait_previous,omitempty"`
	RunMode              *string `json:"run_mode,omitempty" enum:"poll,once,watch"`
	RunOn                *string `json:"run_on,omitempty" desc:"runner label, or a,b = first online"`
}

func (s stepIn) body() map[string]any {
	b := map[string]any{"title": s.Title, "description": "", "progress_total": 1, "progress_current": 0}
	if s.Description != nil {
		b["description"] = *s.Description
	}
	if s.ProgressTotal != nil {
		b["progress_total"] = max(1, *s.ProgressTotal)
	}
	if s.ProgressCurrent != nil {
		b["progress_current"] = max(0, *s.ProgressCurrent)
	}
	s.patchInto(b)
	return b
}

// patchInto adds the optional fields that are set (shared by create and update).
func (s stepIn) patchInto(b map[string]any) {
	set := func(k string, v any, ok bool) {
		if ok {
			b[k] = v
		}
	}
	set("sort_order", deref(s.SortOrder), s.SortOrder != nil)
	set("check_command", deref(s.CheckCommand), s.CheckCommand != nil)
	set("check_interval_seconds", deref(s.CheckIntervalSeconds), s.CheckIntervalSeconds != nil)
	set("wait_previous", deref(s.WaitPrevious), s.WaitPrevious != nil)
	set("run_mode", deref(s.RunMode), s.RunMode != nil)
	set("run_on", deref(s.RunOn), s.RunOn != nil)
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

func checkStatus(s string) (string, error) {
	st := strings.ToLower(strings.TrimSpace(s))
	for _, v := range strings.Split(statusEnum, ",") {
		if st == v {
			return st, nil
		}
	}
	return "", &Err{422, fmt.Sprintf("bad status %q; expected one of %s", s, statusEnum)}
}

type questMutation struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	ProgressLabel string `json:"progress_label"`
	Steps         []struct {
		ID int64 `json:"id"`
	} `json:"steps"`
}

func (q questMutation) result(ctx context.Context, c *Client, withNewStep bool) map[string]any {
	extra := map[string]any{"status": q.Status, "progress_label": q.ProgressLabel}
	if withNewStep && len(q.Steps) > 0 {
		// A step is appended at the end unless sort_order says otherwise; its id is what the caller needs next.
		extra["last_step_id"] = q.Steps[len(q.Steps)-1].ID
	}
	return ok(ctx, c, q.ID, extra)
}

func questTools(c *Client) []Tool {
	type listArgs struct {
		Status    string `json:"status,omitempty" desc:"comma list of statuses; default active,expired (live work). History: completed,failed,archived"`
		Questline string `json:"questline,omitempty" desc:"id or name"`
		Pinned    *bool  `json:"pinned,omitempty"`
		Limit     int    `json:"limit,omitempty" desc:"default 20, max 100"`
		Cursor    string `json:"cursor,omitempty" desc:"next from the previous page"`
	}
	list := Typed("list_quests", "Compact page of quests (no descriptions/steps): id, title, status, significance, progress, deadline, next_step{id,title}, description_len. Newest update first. Details: get_quest.",
		func(ctx context.Context, a listArgs) (any, error) {
			status := a.Status
			if strings.TrimSpace(status) == "" {
				status = "active,expired"
			}
			q := values("summary", "1", "status", status, "limit", fmt.Sprint(clampLimit(a.Limit, 20)), "cursor", a.Cursor)
			if a.Pinned != nil {
				q.Set("pinned", fmt.Sprint(*a.Pinned))
			}
			if a.Questline != "" {
				id, err := resolveID(ctx, c, "/api/questlines", a.Questline, "title")
				if err != nil {
					return nil, err
				}
				if id != nil {
					q.Set("questline_id", itoa(*id))
				}
			}
			page, err := getPage(ctx, c, "/api/quests", q)
			if err != nil {
				return nil, err
			}
			page["workspace"] = c.Workspace(ctx)
			return page, nil
		})

	type getArgs struct {
		ID       int64    `json:"id"`
		Include  []string `json:"include,omitempty" desc:"any of: steps (default), attachments, links, step_text"`
		MaxChars int      `json:"max_chars,omitempty" desc:"description cut, default 4000"`
		Offset   int      `json:"offset,omitempty" desc:"rune offset to continue a cut description"`
	}
	get := Typed("get_quest", "One quest: fields, description (cut at max_chars, continue with offset), steps as summaries (id,title,done,progress). include=step_text adds step descriptions; attachments = file metadata; links = refs it mentions.",
		func(ctx context.Context, a getArgs) (any, error) {
			var q map[string]any
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/quests/%d", a.ID), nil, nil, &q); err != nil {
				return nil, err
			}
			inc := includeSet(a.Include, "steps")
			out := map[string]any{}
			for _, k := range []string{"id", "title", "status", "significance", "pinned", "overlay_hidden", "automated", "failure_inbox",
				"deadline_at", "duration_seconds", "category_id", "category_slug", "questline_id", "questline_title",
				"created_at", "updated_at", "completed_at", "progress_label", "steps_done", "steps_total", "tags"} {
				if v, ok := q[k]; ok {
					out[k] = v
				}
			}
			withText(out, "description", str(q["description"]), a.MaxChars, a.Offset)
			if inc["steps"] || inc["step_text"] {
				out["steps"] = stepSummaries(q["steps"], inc["step_text"], a.MaxChars)
			}
			if inc["links"] {
				out["refs"] = q["refs"]
			}
			if inc["attachments"] {
				var rows []map[string]any
				if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/quests/%d/attachments", a.ID), values("stat", "0"), nil, &rows); err == nil {
					out["attachments"] = attachmentBrief(rows)
				}
			}
			out["workspace"] = c.Workspace(ctx)
			return out, nil
		})

	type stepGet struct {
		ID       int64 `json:"id" desc:"step id"`
		MaxChars int   `json:"max_chars,omitempty"`
		Offset   int   `json:"offset,omitempty"`
	}
	getStep := Typed("get_step", "One step with its full description and check settings, plus a brief of its quest.",
		func(ctx context.Context, a stepGet) (any, error) {
			var resp struct {
				Step  map[string]any `json:"step"`
				Quest map[string]any `json:"quest"`
			}
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/steps/%d", a.ID), nil, nil, &resp); err != nil {
				return nil, err
			}
			step := map[string]any{}
			for k, v := range resp.Step {
				if k != "description" {
					step[k] = v
				}
			}
			withText(step, "description", str(resp.Step["description"]), a.MaxChars, a.Offset)
			return map[string]any{"step": step, "quest": resp.Quest, "workspace": c.Workspace(ctx)}, nil
		})

	type createArgs struct {
		Title           string   `json:"title" desc:"plain text"`
		Description     *string  `json:"description,omitempty" desc:"markdown"`
		Status          string   `json:"status,omitempty" enum:"active,expired,frozen,completed,failed,archived"`
		Significance    string   `json:"significance,omitempty" enum:"insignificant,common,uncommon,epic,legendary"`
		Pinned          *bool    `json:"pinned,omitempty" desc:"always on the HUD; excludes overlay_hidden"`
		OverlayHidden   *bool    `json:"overlay_hidden,omitempty" desc:"never on the HUD"`
		Automated       *bool    `json:"automated,omitempty" desc:"fewer overlay toasts"`
		SortOrder       *int     `json:"sort_order,omitempty"`
		DeadlineAt      *string  `json:"deadline_at,omitempty" desc:"ISO UTC, e.g. 2026-08-20T10:20:00Z"`
		DurationSeconds *int     `json:"duration_seconds,omitempty" desc:"reminder window that opens this long before deadline_at"`
		Category        string   `json:"category,omitempty" desc:"id or name; ignored when questline is given (it inherits)"`
		Questline       string   `json:"questline" desc:"REQUIRED: id or name of the project this quest belongs to (list_questlines), or \"none\" to deliberately leave it standalone"`
		Steps           []stepIn `json:"steps,omitempty"`
		TagIDs          []int64  `json:"tag_ids,omitempty" desc:"pick from list_tags, max 5; a quest without tags is usually a miss"`
		Quiet           *bool    `json:"quiet,omitempty" desc:"default true: no overlay toast"`
	}
	create := Typed("create_quest", "Create a quest (optionally with steps). Returns {id, ok, workspace, status, progress_label}.",
		func(ctx context.Context, a createArgs) (any, error) {
			if strings.TrimSpace(a.Questline) == "" {
				return nil, &Err{422, `questline is required: pass the id or name of the project (see list_questlines) or "none" to leave the quest standalone on purpose`}
			}
			body := map[string]any{"title": a.Title}
			if a.Description != nil {
				body["description"] = *a.Description
			}
			if a.Status != "" {
				st, err := checkStatus(a.Status)
				if err != nil {
					return nil, err
				}
				body["status"] = st
			}
			if a.Significance != "" {
				body["significance"] = a.Significance
			}
			for k, v := range map[string]*bool{"pinned": a.Pinned, "overlay_hidden": a.OverlayHidden, "automated": a.Automated} {
				if v != nil {
					body[k] = *v
				}
			}
			if a.SortOrder != nil {
				body["sort_order"] = *a.SortOrder
			}
			if a.DeadlineAt != nil {
				body["deadline_at"] = *a.DeadlineAt
			}
			if a.DurationSeconds != nil {
				body["duration_seconds"] = *a.DurationSeconds
			}
			ql, err := resolveID(ctx, c, "/api/questlines", a.Questline, "title")
			if err != nil {
				return nil, err
			}
			if ql != nil {
				body["questline_id"] = *ql
			} else if cat, err := resolveID(ctx, c, "/api/categories", a.Category, "slug", "label"); err != nil {
				return nil, err
			} else if cat != nil {
				body["category_id"] = *cat
			}
			if len(a.Steps) > 0 {
				steps := make([]map[string]any, 0, len(a.Steps))
				for _, s := range a.Steps {
					steps = append(steps, s.body())
				}
				body["steps"] = steps
			}
			if a.TagIDs != nil {
				body["tag_ids"] = a.TagIDs
			}
			var q questMutation
			if err := c.Do(ctx, http.MethodPost, "/api/quests", mutationQuery(a.Quiet), body, &q); err != nil {
				return nil, err
			}
			res := q.result(ctx, c, false)
			var hints []string
			if ql == nil && body["category_id"] == nil {
				hints = append(hints, "no questline and no category: the quest is unsorted; set a category with update_quest (category_id) or move it into a questline (questline_id)")
			}
			if len(a.TagIDs) == 0 {
				hints = append(hints, "no tags: pick from list_tags and set them with set_tags (owner quest=N)")
			}
			if len(hints) > 0 {
				res["hint"] = strings.Join(hints, "; ")
			}
			return res, nil
		})

	type updateArgs struct {
		ID              int64   `json:"id"`
		Title           *string `json:"title,omitempty"`
		Description     *string `json:"description,omitempty" desc:"markdown; replaces the whole text"`
		Status          string  `json:"status,omitempty" enum:"active,expired,frozen,completed,failed,archived"`
		Significance    string  `json:"significance,omitempty" enum:"insignificant,common,uncommon,epic,legendary"`
		Pinned          *bool   `json:"pinned,omitempty"`
		OverlayHidden   *bool   `json:"overlay_hidden,omitempty"`
		Automated       *bool   `json:"automated,omitempty"`
		SortOrder       *int    `json:"sort_order,omitempty"`
		DeadlineAt      *string `json:"deadline_at,omitempty"`
		DurationSeconds *int    `json:"duration_seconds,omitempty"`
		CategoryID      *int64  `json:"category_id,omitempty"`
		QuestlineID     *int64  `json:"questline_id,omitempty"`
		ClearQuestline  bool    `json:"clear_questline,omitempty"`
		TagIDs          []int64 `json:"tag_ids,omitempty" desc:"replaces the tag set, max 5"`
		Quiet           *bool   `json:"quiet,omitempty"`
	}
	update := Typed("update_quest", "Change a quest's fields or lifecycle (status). Only given fields change. Steps: add_step/update_step/update_steps.",
		func(ctx context.Context, a updateArgs) (any, error) {
			body := map[string]any{}
			if a.Title != nil {
				body["title"] = *a.Title
			}
			if a.Description != nil {
				body["description"] = *a.Description
			}
			if a.Status != "" {
				st, err := checkStatus(a.Status)
				if err != nil {
					return nil, err
				}
				body["status"] = st
			}
			if a.Significance != "" {
				body["significance"] = a.Significance
			}
			for k, v := range map[string]*bool{"pinned": a.Pinned, "overlay_hidden": a.OverlayHidden, "automated": a.Automated} {
				if v != nil {
					body[k] = *v
				}
			}
			if a.SortOrder != nil {
				body["sort_order"] = *a.SortOrder
			}
			if a.DeadlineAt != nil {
				body["deadline_at"] = *a.DeadlineAt
			}
			if a.DurationSeconds != nil {
				body["duration_seconds"] = *a.DurationSeconds
			}
			if a.CategoryID != nil {
				body["category_id"] = *a.CategoryID
			}
			if a.ClearQuestline {
				body["questline_id"] = nil
			} else if a.QuestlineID != nil {
				body["questline_id"] = *a.QuestlineID
			}
			if a.TagIDs != nil {
				body["tag_ids"] = a.TagIDs
			}
			if len(body) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			var q questMutation
			if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/quests/%d", a.ID), mutationQuery(a.Quiet), body, &q); err != nil {
				return nil, err
			}
			return q.result(ctx, c, false), nil
		})

	type pingArgs struct {
		Message string  `json:"message" desc:"becomes the quest title"`
		Details *string `json:"details,omitempty"`
	}
	ping := Typed("ping_user", "Interrupt the user via the HUD (a pinned legendary quest, never quiet) when you are blocked on their answer and they may be away. Not for progress updates.",
		func(ctx context.Context, a pingArgs) (any, error) {
			body := map[string]any{"title": a.Message, "pinned": true, "significance": "legendary"}
			if a.Details != nil {
				body["description"] = *a.Details
			}
			var q questMutation
			f := false
			if err := c.Do(ctx, http.MethodPost, "/api/quests", mutationQuery(&f), body, &q); err != nil {
				return nil, err
			}
			return q.result(ctx, c, false), nil
		})

	return []Tool{list, get, getStep, create, update, ping}
}

func includeSet(inc []string, defaults ...string) map[string]bool {
	m := map[string]bool{}
	if len(inc) == 0 {
		for _, d := range defaults {
			m[d] = true
		}
		return m
	}
	for _, i := range inc {
		m[strings.ToLower(strings.TrimSpace(i))] = true
	}
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// stepSummaries reduces steps to what is needed to pick one; withText adds each step's (cut) description.
func stepSummaries(raw any, withBody bool, maxChars int) []map[string]any {
	steps, _ := raw.([]any)
	out := make([]map[string]any, 0, len(steps))
	for _, r := range steps {
		s, _ := r.(map[string]any)
		row := map[string]any{
			"id": s["id"], "title": s["title"], "done": s["done"],
			"progress_current": s["progress_current"], "progress_total": s["progress_total"],
		}
		d := str(s["description"])
		row["description_len"] = len([]rune(d))
		if withBody {
			t, _, _ := text(d, maxChars, 0)
			row["description"] = t
		}
		out = append(out, row)
	}
	return out
}

func attachmentBrief(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		if h, _ := a["hidden"].(bool); h {
			continue
		}
		ct := a["content_type_detected"]
		if ct == nil || ct == "" {
			ct = a["content_type_declared"]
		}
		out = append(out, map[string]any{
			"id": a["id"], "filename": a["filename"], "size_bytes": a["size_bytes"], "content_type": ct,
			"scan_status": a["scan_status"], "comment": a["comment"], "available": a["available"],
			"source_updated": a["source_updated"],
		})
	}
	return out
}
