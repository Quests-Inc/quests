package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Layout state is shared with the web map, including tree settings and annotations.
func graphLayoutTool(c *Client) Tool {
	type args struct {
		Action  string         `json:"action" enum:"list,get,create,diff,update,delete"`
		ID      int64          `json:"id,omitempty" desc:"get/diff/update/delete: saved layout id"`
		Title   string         `json:"title,omitempty" desc:"create: title; update: optional rename"`
		State   map[string]any `json:"state,omitempty" desc:"Full web layout JSON: v:1, center:{type,id}, roots:[{type,id}], trees:{'quest:5':{depth:2,layout:'tree'}}, depth:2, layout:'tree', statuses:['active'], types:['quest','step','note','questline','attachment'], hidden:[], expand:[], folds:{expanded:[],loose:[]}, positions:{'quest:5':{x:0,y:0}}, entities:[{key:'quest:5',title:'Name'}], sizes:{}, zones:[{id:'unique',title:'Area',color:'#6a8fbf',x:0,y:0,w:600,h:400}], view:{cx:0,cy:0,k:1}. Preserve fields read with get. Modes: tree,down,radial,layers,free; tree depth 1..3. entities is the visible saved membership, excluding draft and cluster cards. World-space zones are independent of cards; min w/h 40, max 200 zones."`
		Confirm bool           `json:"confirm,omitempty" desc:"update with state or delete: only true after user approval of the diff/deletion; false returns preview without writing"`
	}
	return Typed("graph_layout", "Personal saved map layouts in the active workspace. list/get read; create stores full state; diff compares proposed state with the saved entity manifest (added/removed). update with state previews by default: show the diff to the user before confirm=true. Rename without state needs no confirmation. delete also previews unless confirmed. Trees have independent depth/layout. Zones are labelled coloured rectangles, stored in state.zones. Old layouts without entities infer membership from positions/roots (legacy=true). This edits saved layouts, not the live browser canvas. See docs/graph-map.md for state format.", func(ctx context.Context, a args) (any, error) {
		if a.Action != "list" && a.Action != "create" && a.ID < 1 {
			return nil, &Err{422, "id is required"}
		}
		path := "/api/graph-layouts"
		if a.ID > 0 && a.Action != "list" {
			path += "/" + itoa(a.ID)
		}
		switch a.Action {
		case "list", "get":
			var out any
			if err := c.Do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
				return nil, err
			}
			return map[string]any{"data": out, "workspace": c.Workspace(ctx)}, nil
		case "create":
			if strings.TrimSpace(a.Title) == "" || a.State == nil {
				return nil, &Err{422, "create needs title and state"}
			}
			var out any
			if err := c.Do(ctx, http.MethodPost, "/api/graph-layouts", nil, map[string]any{"title": a.Title, "state": a.State}, &out); err != nil {
				return nil, err
			}
			return map[string]any{"data": out, "workspace": c.Workspace(ctx)}, nil
		case "diff", "update", "delete":
			var previous struct {
				Title string         `json:"title"`
				State map[string]any `json:"state"`
			}
			if err := c.Do(ctx, http.MethodGet, path, nil, nil, &previous); err != nil {
				return nil, err
			}
			if a.Action == "delete" {
				if !a.Confirm {
					return map[string]any{"requires_confirmation": true, "id": a.ID, "title": previous.Title, "workspace": c.Workspace(ctx)}, nil
				}
				if err := c.Do(ctx, http.MethodDelete, path, nil, nil, nil); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "workspace": c.Workspace(ctx)}, nil
			}
			if a.Action == "diff" && a.State == nil {
				return nil, &Err{422, "diff needs state"}
			}
			if a.State != nil {
				added, removed, legacy := graphLayoutDiff(previous.State, a.State)
				if a.Action == "diff" || !a.Confirm {
					return map[string]any{"id": a.ID, "title": previous.Title, "added": added, "removed": removed, "legacy": legacy, "requires_confirmation": a.Action == "update", "workspace": c.Workspace(ctx)}, nil
				}
			}
			body := map[string]any{}
			if a.Title != "" {
				body["title"] = a.Title
			}
			if a.State != nil {
				body["state"] = a.State
			}
			if len(body) == 0 {
				return nil, &Err{422, "update needs title or state"}
			}
			var out any
			if err := c.Do(ctx, http.MethodPatch, path, nil, body, &out); err != nil {
				return nil, err
			}
			return map[string]any{"data": out, "workspace": c.Workspace(ctx)}, nil
		default:
			return nil, &Err{422, "unknown action"}
		}
	})
}

type layoutEntity struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

var layoutEntityKey = regexp.MustCompile(`^(quest|step|note|questline|attachment|template):[1-9][0-9]*$`)

func graphLayoutEntities(state map[string]any) (map[string]layoutEntity, bool) {
	// Decode typed fields without assuming the caller's in-memory JSON number types.
	data, _ := json.Marshal(state)
	var s struct {
		Entities  *[]layoutEntity `json:"entities"`
		Positions map[string]any  `json:"positions"`
		Hidden    []string        `json:"hidden"`
		Center    struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		} `json:"center"`
		Roots []struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		} `json:"roots"`
	}
	_ = json.Unmarshal(data, &s)
	out := map[string]layoutEntity{}
	add := func(e layoutEntity) {
		if layoutEntityKey.MatchString(e.Key) {
			if e.Title == "" {
				e.Title = e.Key
			}
			out[e.Key] = e
		}
	}
	if s.Entities != nil {
		for _, e := range *s.Entities {
			add(e)
		}
		return out, false
	}
	hidden := map[string]bool{}
	for _, k := range s.Hidden {
		hidden[k] = true
	}
	for k := range s.Positions {
		if !hidden[k] {
			add(layoutEntity{k, k})
		}
	}
	if s.Center.Type != "" && s.Center.ID > 0 {
		key := s.Center.Type + ":" + itoa(s.Center.ID)
		add(layoutEntity{key, key})
	}
	for _, r := range s.Roots {
		key := r.Type + ":" + itoa(r.ID)
		add(layoutEntity{key, key})
	}
	return out, true
}

func graphLayoutDiff(before, after map[string]any) (added, removed []layoutEntity, legacy bool) {
	old, legacy := graphLayoutEntities(before)
	next, _ := graphLayoutEntities(after)
	added, removed = []layoutEntity{}, []layoutEntity{}
	for k, e := range next {
		if _, ok := old[k]; !ok {
			added = append(added, e)
		}
	}
	for k, e := range old {
		if _, ok := next[k]; !ok {
			removed = append(removed, e)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Key < added[j].Key })
	sort.Slice(removed, func(i, j int) bool { return removed[i].Key < removed[j].Key })
	return
}
