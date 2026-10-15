package mcp

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func noteTools(c *Client) []Tool {
	type listArgs struct {
		ParentID   *int64 `json:"parent_id,omitempty" desc:"children of this note"`
		CategoryID *int64 `json:"category_id,omitempty" desc:"everything below this CATEGORY note"`
		Flags      string `json:"flags,omitempty" desc:"comma list; minus = without: outdated, readme, private, category, pinned, check (e.g. -outdated,-private)"`
		Sort       string `json:"sort,omitempty" enum:"tree,popular"`
		Limit      int    `json:"limit,omitempty" desc:"default 20, max 100"`
		Cursor     string `json:"cursor,omitempty" desc:"next from the previous page"`
	}
	list := Typed("list_notes", "Page of notes without their text (id, title, parent, flags, description_len). Read one with get_note.",
		func(ctx context.Context, a listArgs) (any, error) {
			q := values("brief", "1", "limit", fmt.Sprint(clampLimit(a.Limit, 20)), "cursor", a.Cursor, "flags", a.Flags)
			if a.ParentID != nil {
				q.Set("parent_id", itoa(*a.ParentID))
			}
			if a.CategoryID != nil {
				q.Set("category_id", itoa(*a.CategoryID))
			}
			if a.Sort == "popular" {
				q.Set("sort", "popular")
			}
			page, err := getPage(ctx, c, "/api/notes", q)
			if err != nil {
				return nil, err
			}
			page["workspace"] = c.Workspace(ctx)
			return page, nil
		})

	type getArgs struct {
		ID       int64 `json:"id"`
		MaxChars int   `json:"max_chars,omitempty" desc:"text cut, default 4000"`
		Offset   int   `json:"offset,omitempty" desc:"rune offset to continue a cut text"`
	}
	get := Typed("get_note", "One note: text (cut at max_chars, continue with offset), refs it cites, backlinks, children, outdated flag/check. A PRIVATE note comes back with a placeholder instead of its text: ask the user, do not work around it.",
		func(ctx context.Context, a getArgs) (any, error) {
			var resp struct {
				Note   map[string]any   `json:"note"`
				Quests []map[string]any `json:"quests"`
			}
			if err := c.Do(ctx, http.MethodGet, "/api/context", values("note", itoa(a.ID)), nil, &resp); err != nil {
				return nil, err
			}
			n := resp.Note
			if n == nil {
				return nil, &Err{404, fmt.Sprintf("note=%d not found", a.ID)}
			}
			out := map[string]any{}
			for k, v := range n {
				if k != "description" && k != "attachments" {
					out[k] = v
				}
			}
			withText(out, "description", str(n["description"]), a.MaxChars, a.Offset)
			if rows, ok := n["attachments"].([]any); ok {
				var list []map[string]any
				for _, r := range rows {
					if m, ok := r.(map[string]any); ok {
						list = append(list, m)
					}
				}
				out["attachments"] = attachmentBrief(list)
			}
			// quests that cite this note: just enough to open them
			cited := make([]map[string]any, 0, len(resp.Quests))
			for _, q := range resp.Quests {
				cited = append(cited, map[string]any{"id": q["id"], "title": q["title"], "status": q["status"]})
			}
			out["quests"] = cited
			out["workspace"] = c.Workspace(ctx)
			return out, nil
		})

	type searchArgs struct {
		Q     string `json:"q" desc:"fuzzy: tolerates wrong keyboard layout and typos; \"exact phrase\"; -excluded"`
		Limit int    `json:"limit,omitempty" desc:"default 10, max 50"`
		Flags string `json:"flags,omitempty" desc:"as in list_notes, e.g. -outdated"`
	}
	search := Typed("search_notes", "Search the notes vault: ranked hits with snippets.",
		func(ctx context.Context, a searchArgs) (any, error) {
			if strings.TrimSpace(a.Q) == "" {
				return nil, &Err{422, "q is empty"}
			}
			lim := a.Limit
			if lim <= 0 {
				lim = 10
			}
			if lim > 50 {
				lim = 50
			}
			var res map[string]any
			if err := c.Do(ctx, http.MethodGet, "/api/notes/search", values("q", a.Q, "limit", fmt.Sprint(lim), "flags", strings.TrimSpace(a.Flags)), nil, &res); err != nil {
				return nil, err
			}
			res["workspace"] = c.Workspace(ctx)
			return res, nil
		})

	type none struct{}
	cats := Typed("list_note_categories", "CATEGORY notes (folders) with path, child count and a hint what belongs in each. create_note needs a parent from here.",
		func(ctx context.Context, _ none) (any, error) {
			rows, err := allNotes(ctx, c)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": categoryRows(rows), "workspace": c.Workspace(ctx)}, nil
		})

	type createArgs struct {
		Title       string  `json:"title"`
		Description *string `json:"description,omitempty" desc:"markdown; images only via upload_attachment + ![alt](attachment=N)"`
		ParentID    *int64  `json:"parent_id,omitempty" desc:"a CATEGORY note or any note"`
		Pinned      *bool   `json:"pinned,omitempty"`
		IsReadme    *bool   `json:"is_readme,omitempty"`
		IsPrivate   *bool   `json:"is_private,omitempty" desc:"hidden from MCP readers"`
		IsCategory  *bool   `json:"is_category,omitempty"`
		SortOrder   *int    `json:"sort_order,omitempty"`
		NoCategory  bool    `json:"no_category,omitempty" desc:"allow a top-level note although categories exist"`
	}
	create := Typed("create_note", "Create a note. Without parent_id it is refused while CATEGORY notes exist: pick one (list_note_categories) or pass no_category.",
		func(ctx context.Context, a createArgs) (any, error) {
			if a.ParentID == nil && (a.IsCategory == nil || !*a.IsCategory) && !a.NoCategory {
				rows, err := allNotes(ctx, c)
				if err != nil {
					return nil, err
				}
				if cats := categoryRows(rows); len(cats) > 0 {
					return nil, &Err{422, categoryRefusal(cats)}
				}
			}
			body := map[string]any{"title": a.Title}
			setIf(body, "description", a.Description)
			setIf(body, "parent_id", a.ParentID)
			setIf(body, "pinned", a.Pinned)
			setIf(body, "is_readme", a.IsReadme)
			setIf(body, "is_private", a.IsPrivate)
			setIf(body, "is_category", a.IsCategory)
			setIf(body, "sort_order", a.SortOrder)
			var n struct {
				ID int64 `json:"id"`
			}
			if err := c.Do(ctx, http.MethodPost, "/api/notes", nil, body, &n); err != nil {
				return nil, err
			}
			return ok(ctx, c, n.ID, nil), nil
		})

	type updArgs struct {
		ID           int64   `json:"id"`
		Title        *string `json:"title,omitempty"`
		Description  *string `json:"description,omitempty" desc:"replace the whole text (one text op per call)"`
		InsertText   *string `json:"insert_text,omitempty" desc:"insert; position: insert_after / insert_before (a unique snippet) or insert_at (rune offset); default end"`
		InsertAfter  *string `json:"insert_after,omitempty"`
		InsertBefore *string `json:"insert_before,omitempty"`
		InsertAt     *int    `json:"insert_at,omitempty"`
		ReplaceFind  *string `json:"replace_find,omitempty" desc:"replace this snippet (unique unless replace_all) with replace_with"`
		ReplaceWith  *string `json:"replace_with,omitempty"`
		ReplaceAll   bool    `json:"replace_all,omitempty"`
		ParentID     *int64  `json:"parent_id,omitempty"`
		ClearParent  bool    `json:"clear_parent,omitempty"`
		SortOrder    *int    `json:"sort_order,omitempty"`
	}
	update := Typed("update_note", "Edit a note: title, text (replace whole / insert / find-and-replace), parent, order. Flags (pinned, readme, private, category, outdated): set_note_flag.",
		func(ctx context.Context, a updArgs) (any, error) {
			body := map[string]any{}
			setIf(body, "title", a.Title)
			ops := 0
			for _, on := range []bool{a.Description != nil, a.InsertText != nil, a.ReplaceFind != nil} {
				if on {
					ops++
				}
			}
			if ops > 1 {
				return nil, &Err{422, "use one text operation per call: description, insert_text or replace_find"}
			}
			setIf(body, "description", a.Description)
			if a.InsertText != nil {
				op := map[string]any{"text": *a.InsertText}
				pos := 0
				for k, v := range map[string]any{"after": a.InsertAfter, "before": a.InsertBefore, "at": a.InsertAt} {
					switch p := v.(type) {
					case *string:
						if p != nil {
							op[k] = *p
							pos++
						}
					case *int:
						if p != nil {
							op[k] = *p
							pos++
						}
					}
				}
				if pos > 1 {
					return nil, &Err{422, "provide at most one of: insert_after, insert_before, insert_at"}
				}
				body["description_insert"] = op
			}
			if a.ReplaceFind != nil {
				if a.ReplaceWith == nil {
					return nil, &Err{422, "replace_with is required when replace_find is set"}
				}
				body["description_replace"] = map[string]any{"find": *a.ReplaceFind, "with": *a.ReplaceWith, "all": a.ReplaceAll}
			}
			if a.ClearParent {
				body["parent_id"] = nil
			} else {
				setIf(body, "parent_id", a.ParentID)
			}
			setIf(body, "sort_order", a.SortOrder)
			if len(body) == 0 {
				return nil, &Err{422, "provide at least one field to update"}
			}
			if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/notes/%d", a.ID), nil, body, nil); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, nil), nil
		})

	type idArg struct {
		ID int64 `json:"id"`
	}
	del := Typed("delete_note", "Delete a note.", func(ctx context.Context, a idArg) (any, error) {
		if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/notes/%d", a.ID), nil, nil, nil); err != nil {
			return nil, err
		}
		return ok(ctx, c, a.ID, map[string]any{"deleted": true}), nil
	})

	type flagArgs struct {
		ID     int64  `json:"id"`
		Flag   string `json:"flag" enum:"outdated,pinned,readme,private,category"`
		Value  *bool  `json:"value,omitempty" desc:"default true"`
		Reason string `json:"reason,omitempty" desc:"for outdated: why the note is stale"`
	}
	flag := Typed("set_note_flag", "Set or clear a note flag: outdated (stale; give a reason), pinned, readme, private (hidden from MCP), category (folder).",
		func(ctx context.Context, a flagArgs) (any, error) {
			on := a.Value == nil || *a.Value
			var err error
			switch a.Flag {
			case "outdated":
				err = c.Do(ctx, http.MethodPut, fmt.Sprintf("/api/notes/%d/outdated", a.ID), nil, map[string]any{"outdated": on, "reason": a.Reason}, nil)
			case "pinned", "readme", "private", "category":
				field := map[string]string{"pinned": "pinned", "readme": "is_readme", "private": "is_private", "category": "is_category"}[a.Flag]
				err = c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/notes/%d", a.ID), nil, map[string]any{field: on}, nil)
			default:
				err = &Err{422, fmt.Sprintf("unknown flag %q", a.Flag)}
			}
			if err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"flag": a.Flag, "value": on}), nil
		})

	type rateArgs struct {
		ID     int64 `json:"id"`
		Useful bool  `json:"useful"`
	}
	rate := Typed("rate_note", "Tell the vault whether a note helped (feeds the popularity order).", func(ctx context.Context, a rateArgs) (any, error) {
		if err := c.Do(ctx, http.MethodPost, fmt.Sprintf("/api/notes/%d/feedback", a.ID), nil, map[string]any{"useful": a.Useful}, nil); err != nil {
			return nil, err
		}
		return ok(ctx, c, a.ID, nil), nil
	})

	type checkArgs struct {
		ID              int64   `json:"id"`
		Command         *string `json:"command,omitempty" desc:"freshness check; last output line 0|1 (1 = outdated) or JSON {outdated,reason}; empty string removes the check"`
		IntervalSeconds *int    `json:"interval_seconds,omitempty" desc:"default 3600, min 15"`
		RunOn           *string `json:"run_on,omitempty" desc:"runner label (required with command)"`
	}
	check := Typed("set_note_check", "Give a note a freshness check (runs on a runner every interval), or remove it with an empty command. Owner-only.",
		func(ctx context.Context, a checkArgs) (any, error) {
			body := map[string]any{"command": a.Command, "interval_seconds": a.IntervalSeconds, "run_on": a.RunOn}
			var res map[string]any
			if err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/api/notes/%d/check", a.ID), nil, body, &res); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"check": res}), nil
		})
	runNow := Typed("check_note_now", "Run a note's freshness check right away.", func(ctx context.Context, a idArg) (any, error) {
		var res map[string]any
		if err := c.Do(ctx, http.MethodPost, fmt.Sprintf("/api/notes/%d/check/run", a.ID), nil, nil, &res); err != nil {
			return nil, err
		}
		return ok(ctx, c, a.ID, map[string]any{"check": res}), nil
	})

	return []Tool{list, get, search, cats, create, update, del, flag, rate, check, runNow}
}

// setIf puts v under key when the pointer is set.
func setIf[T any](m map[string]any, key string, p *T) {
	if p != nil {
		m[key] = *p
	}
}

// allNotes loads the whole vault tree (titles/flags matter here, texts are ignored): used for categories only.
func allNotes(ctx context.Context, c *Client) ([]map[string]any, error) {
	var rows []map[string]any
	if err := c.Do(ctx, http.MethodGet, "/api/notes", nil, nil, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

type categoryRow struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	ParentID *int64 `json:"parent_id"`
	Path     string `json:"path"`
	Children int    `json:"children"`
	Hint     string `json:"hint"`
}

func categoryRows(rows []map[string]any) []categoryRow {
	byID := map[int64]map[string]any{}
	kids := map[int64]int{}
	for _, n := range rows {
		byID[int64(num(n["id"]))] = n
		if p, ok := n["parent_id"].(float64); ok {
			kids[int64(p)]++
		}
	}
	var out []categoryRow
	for _, n := range rows {
		if b, _ := n["is_category"].(bool); !b {
			continue
		}
		id := int64(num(n["id"]))
		r := categoryRow{ID: id, Title: str(n["title"]), Children: kids[id], Hint: categoryHint(n)}
		if p, ok := n["parent_id"].(float64); ok {
			pid := int64(p)
			r.ParentID = &pid
		}
		parts := []string{r.Title}
		seen := map[int64]bool{id: true}
		for p := r.ParentID; p != nil && !seen[*p]; {
			seen[*p] = true
			par, ok := byID[*p]
			if !ok {
				break
			}
			parts = append(parts, str(par["title"]))
			if pp, ok := par["parent_id"].(float64); ok {
				v := int64(pp)
				p = &v
			} else {
				p = nil
			}
		}
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		r.Path = strings.Join(parts, " / ")
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// categoryHint is the first non-empty line of a category's text: what belongs in it.
func categoryHint(n map[string]any) string {
	if p, _ := n["is_private"].(bool); p {
		return ""
	}
	for _, line := range strings.Split(str(n["description"]), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#>*- "))
		if line != "" {
			r := []rune(line)
			if len(r) > 200 {
				r = r[:200]
			}
			return string(r)
		}
	}
	return ""
}

func categoryRefusal(cats []categoryRow) string {
	var b strings.Builder
	b.WriteString("create_note refused: the vault has CATEGORY notes (folders); pass parent_id=<one of them or a note under it>, or no_category=true if none fits:\n")
	for _, c := range cats {
		hint := ""
		if c.Hint != "" {
			hint = " — " + c.Hint
		}
		fmt.Fprintf(&b, "  note=%d %s (%d children)%s\n", c.ID, c.Path, c.Children, hint)
	}
	return strings.TrimRight(b.String(), "\n")
}
