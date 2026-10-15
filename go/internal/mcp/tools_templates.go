package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// templateFields is a template's editable part; unset (nil) fields are not sent.
type templateFields struct {
	Title           *string        `json:"title,omitempty"`
	Description     *string        `json:"description,omitempty" desc:"markdown"`
	Freq            *string        `json:"freq,omitempty" enum:"daily,weekly"`
	Weekdays        *string        `json:"weekdays,omitempty" desc:"weekly: comma list, 0=Mon..6=Sun"`
	Timezone        *string        `json:"timezone,omitempty"`
	EmitMode        *string        `json:"emit_mode,omitempty" enum:"fixed,surprise"`
	EmitChance      *float64       `json:"emit_chance,omitempty"`
	EmitWindowStart *string        `json:"emit_window_start,omitempty"`
	EmitWindowEnd   *string        `json:"emit_window_end,omitempty"`
	EmitPoolCommand *string        `json:"emit_pool_command,omitempty" desc:"shell one-liner or #!script printing ONE quest as JSON; runs on emit_run_on"`
	EmitRunOn       *string        `json:"emit_run_on,omitempty" desc:"runner label (required with a command)"`
	EmitLimits      map[string]int `json:"emit_limits,omitempty" desc:"max_steps, max_title, max_description, max_command"`
	DeadlineTime    *string        `json:"deadline_time,omitempty"`
	DurationSeconds *int           `json:"duration_seconds,omitempty"`
	Significance    *string        `json:"significance,omitempty" enum:"insignificant,common,uncommon,epic,legendary"`
	Pinned          *bool          `json:"pinned,omitempty"`
	OverlayHidden   *bool          `json:"overlay_hidden,omitempty"`
	Enabled         *bool          `json:"enabled,omitempty"`
	Automated       *bool          `json:"automated,omitempty"`
}

// templateBody is the request body of templateFields plus the resolved category/questline and steps.
func templateBody(ctx context.Context, c *Client, f templateFields, category, questline string, steps []stepIn) (map[string]any, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	if id, err := resolveID(ctx, c, "/api/categories", category, "slug", "label"); err != nil {
		return nil, err
	} else if id != nil {
		body["category_id"] = *id
	}
	if id, err := resolveID(ctx, c, "/api/questlines", questline, "title"); err != nil {
		return nil, err
	} else if id != nil {
		body["questline_id"] = *id
	}
	if steps != nil {
		list := make([]map[string]any, 0, len(steps))
		for _, s := range steps {
			list = append(list, s.body())
		}
		body["steps"] = list
	}
	return body, nil
}

func templateTools(c *Client) []Tool {
	type listArgs struct {
		Enabled *bool `json:"enabled,omitempty"`
		Limit   int   `json:"limit,omitempty" desc:"default 10, max 100"`
	}
	list := Typed("list_templates", "Recurring quest templates as summaries (id, title, freq, enabled, emit mode, has_command). Details: get_template.",
		func(ctx context.Context, a listArgs) (any, error) {
			q := url.Values{}
			if a.Enabled != nil {
				q.Set("enabled", map[bool]string{true: "1", false: "0"}[*a.Enabled])
			}
			var rows []map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/templates", q, nil, &rows); err != nil {
				return nil, err
			}
			lim := clampLimit(a.Limit, 10)
			items := make([]map[string]any, 0, min(lim, len(rows)))
			for i, t := range rows {
				if i == lim {
					break
				}
				items = append(items, map[string]any{
					"id": t["id"], "title": t["title"], "freq": t["freq"], "weekdays": t["weekdays"], "enabled": t["enabled"],
					"emit_mode": t["emit_mode"], "emit_run_on": t["emit_run_on"],
					"has_command":  strings.TrimSpace(str(t["emit_pool_command"])) != "",
					"questline_id": t["questline_id"], "significance": t["significance"],
				})
			}
			return map[string]any{"items": items, "total": len(rows), "truncated": len(rows) > lim, "workspace": c.Workspace(ctx)}, nil
		})

	type getArgs struct {
		ID       int64 `json:"id"`
		MaxChars int   `json:"max_chars,omitempty" desc:"cut for description and emit_pool_command, default 4000"`
	}
	get := Typed("get_template", "One template with its steps; long description and emit_pool_command are cut at max_chars.",
		func(ctx context.Context, a getArgs) (any, error) {
			var t map[string]any
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/templates/%d", a.ID), nil, nil, &t); err != nil {
				return nil, err
			}
			for _, k := range []string{"description", "emit_pool_command"} {
				if s, ok := t[k].(string); ok {
					withText(t, k, s, a.MaxChars, 0)
				}
			}
			t["workspace"] = c.Workspace(ctx)
			return t, nil
		})

	type createArgs struct {
		templateFields
		Title     string   `json:"title"`
		Category  string   `json:"category,omitempty" desc:"id or name"`
		Questline string   `json:"questline,omitempty" desc:"id or name"`
		Steps     []stepIn `json:"steps,omitempty"`
	}
	create := Typed("create_template", "Create a recurring template (daily/weekly), optionally emitting its quest from a command.",
		func(ctx context.Context, a createArgs) (any, error) {
			a.templateFields.Title = &a.Title
			body, err := templateBody(ctx, c, a.templateFields, a.Category, a.Questline, a.Steps)
			if err != nil {
				return nil, err
			}
			var t struct {
				ID int64 `json:"id"`
			}
			if err := c.Do(ctx, http.MethodPost, "/api/templates", nil, body, &t); err != nil {
				return nil, err
			}
			return ok(ctx, c, t.ID, nil), nil
		})

	type updArgs struct {
		templateFields
		ID        int64    `json:"id"`
		Category  string   `json:"category,omitempty"`
		Questline string   `json:"questline,omitempty"`
		Steps     []stepIn `json:"steps,omitempty" desc:"replaces the template's steps"`
	}
	upd := Typed("update_template", "Change a template; only given fields change.",
		func(ctx context.Context, a updArgs) (any, error) {
			body, err := templateBody(ctx, c, a.templateFields, a.Category, a.Questline, a.Steps)
			if err != nil {
				return nil, err
			}
			if len(body) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/templates/%d", a.ID), nil, body, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, nil), nil
		})

	type idArg struct {
		ID int64 `json:"id"`
	}
	del := Typed("delete_template", "Delete a template (quests it already made stay).", func(ctx context.Context, a idArg) (any, error) {
		if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/templates/%d", a.ID), nil, nil, nil); err != nil {
			return nil, err
		}
		return ok(ctx, c, a.ID, map[string]any{"deleted": true}), nil
	})

	type attemptsArgs struct {
		TemplateID int64 `json:"template_id"`
		Limit      int   `json:"limit,omitempty" desc:"default 10"`
	}
	attempts := Typed("list_emit_attempts", "Recent emit_pool_command runs of a template (status, duration, message): why a template did or did not emit.",
		func(ctx context.Context, a attemptsArgs) (any, error) {
			var res map[string]any
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/templates/%d/emit-attempts", a.TemplateID), values("limit", fmt.Sprint(clampLimit(a.Limit, 10))), nil, &res); err != nil {
				return nil, err
			}
			res["workspace"] = c.Workspace(ctx)
			return res, nil
		})

	type dryArgs struct {
		Command    *string `json:"command,omitempty" desc:"text to try"`
		TemplateID *int64  `json:"template_id,omitempty" desc:"or: use this template's stored command"`
	}
	dry := Typed("dry_run_emit_pool", "Run an emit_pool_command here, like the scheduler would (20 s limit, no quest created, real side effects of the script do happen) and show the quest it prints. Give command or template_id. A green result does not prove it works on the runner device (other env, other tools).",
		func(ctx context.Context, a dryArgs) (any, error) {
			if (a.Command == nil) == (a.TemplateID == nil) {
				return nil, &Err{422, "provide exactly one of: command, template_id"}
			}
			cmd := ""
			if a.Command != nil {
				cmd = *a.Command
			} else {
				var t map[string]any
				if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/templates/%d", *a.TemplateID), nil, nil, &t); err != nil {
					return nil, err
				}
				cmd = str(t["emit_pool_command"])
				if strings.TrimSpace(cmd) == "" {
					return nil, &Err{422, fmt.Sprintf("template %d has no emit_pool_command set", *a.TemplateID)}
				}
			}
			return runEmitPool(ctx, cmd), nil
		})

	return []Tool{list, get, create, upd, del, attempts, dry}
}

const emitPoolTimeout = 20 * time.Second

// runEmitPool executes a command like the scheduler (shebang → its own interpreter, otherwise sh -c) and parses its stdout.
func runEmitPool(ctx context.Context, command string) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, emitPoolTimeout)
	defer cancel()
	home, _ := os.UserHomeDir()
	var cmd *exec.Cmd
	mode := "sh -c"
	if strings.HasPrefix(strings.TrimLeft(command, " \t\r\n"), "#!") {
		mode = "script"
		f, err := os.CreateTemp("", "quests-emit-pool-")
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		defer os.Remove(f.Name())
		_, werr := f.WriteString(command)
		_ = f.Close()
		if werr != nil || os.Chmod(f.Name(), 0o700) != nil {
			return map[string]any{"ok": false, "error": "cannot write the script"}
		}
		cmd = exec.CommandContext(ctx, f.Name())
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = home
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	trace := map[string]any{"exec_mode": mode, "stdout": clip(stdout.String(), 4000), "stderr": clip(stderr.String(), 4000)}
	if ctx.Err() == context.DeadlineExceeded {
		trace["ok"], trace["error"] = false, fmt.Sprintf("timed out after %s", emitPoolTimeout)
		return trace
	}
	exit := 0
	if err != nil {
		ee, isExit := err.(*exec.ExitError)
		if !isExit {
			trace["ok"], trace["error"] = false, err.Error()
			return trace
		}
		exit = ee.ExitCode()
	}
	trace["exit_code"] = exit
	if exit != 0 {
		trace["ok"], trace["error"] = false, fmt.Sprintf("non-zero exit (%d)", exit)
		return trace
	}
	out := strings.TrimSpace(stdout.String())
	var raw any
	if out != "" {
		if err := json.Unmarshal([]byte(out), &raw); err != nil {
			trace["ok"], trace["error"] = false, "invalid JSON on stdout: "+err.Error()
			return trace
		}
	}
	switch v := raw.(type) {
	case nil:
		trace["ok"], trace["quest"], trace["note"] = true, nil, "no quest this period (null / {} / nothing printed)"
	case map[string]any:
		if len(v) == 0 {
			trace["ok"], trace["quest"], trace["note"] = true, nil, "no quest this period (null / {} / nothing printed)"
			break
		}
		known := map[string]bool{"title": true, "description": true, "significance": true, "pinned": true, "deadline_at": true,
			"duration_seconds": true, "questline": true, "questline_id": true, "category": true, "category_id": true,
			"tags": true, "tag_ids": true, "automated": true, "steps": true}
		var ignored []string
		for k := range v {
			if !known[k] {
				ignored = append(ignored, k)
			}
		}
		steps, _ := v["steps"].([]any)
		trace["ok"], trace["quest"], trace["steps_count"] = true, v, len(steps)
		if len(ignored) > 0 {
			trace["ignored_fields"] = ignored
		}
	case []any:
		trace["ok"], trace["note"] = true, "legacy array format: items become the steps of one quest"
		trace["legacy_items"] = len(v)
	default:
		trace["ok"], trace["error"] = false, "stdout JSON must be a quest object (or null)"
	}
	return trace
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
