package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const defaultMaxChars = 4000

// text cuts s to max runes starting at rune offset off; the second value says how much there is in total.
func text(s string, max, off int) (out string, total int, truncated bool) {
	if max <= 0 {
		max = defaultMaxChars
	}
	r := []rune(s)
	total = len(r)
	if off < 0 {
		off = 0
	}
	if off > total {
		off = total
	}
	end := off + max
	if end >= total {
		return string(r[off:]), total, false
	}
	return string(r[off:end]), total, true
}

// withText adds the description fields of a possibly long text to out.
func withText(out map[string]any, key, s string, max, off int) {
	t, total, trunc := text(s, max, off)
	out[key] = t
	out[key+"_len"] = total
	if trunc || off > 0 {
		out[key+"_truncated"] = trunc
		if trunc {
			out[key+"_next_offset"] = off + utf8.RuneCountInString(t)
		}
	}
}

func values(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return v
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// quiet is the query every mutation sends: it tells the API the call came from MCP and, by default, not to toast.
func mutationQuery(quiet *bool) url.Values {
	v := url.Values{"source": {"mcp"}}
	if quiet == nil || *quiet {
		v.Set("quiet", "1")
	}
	return v
}

// ok is the shape every mutation answers with.
func ok(ctx context.Context, c *Client, id int64, extra map[string]any) map[string]any {
	out := map[string]any{"id": id, "ok": true, "workspace": c.Workspace(ctx)}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// resolveID turns "12", or a name/substring of a title/slug/label, into an id via GET path
// (the same convention as the CLI: exact match wins, otherwise a unique substring).
func resolveID(ctx context.Context, c *Client, path, raw string, fields ...string) (*int64, error) {
	t := strings.TrimSpace(raw)
	switch strings.ToLower(t) {
	case "", "none", "-", "null", "нет":
		return nil, nil
	}
	if n, err := strconv.ParseInt(t, 10, 64); err == nil {
		return &n, nil
	}
	var rows []map[string]any
	if err := c.Do(ctx, "GET", path, nil, nil, &rows); err != nil {
		return nil, err
	}
	needle := strings.ToLower(t)
	var exact, part []int64
	for _, row := range rows {
		id, _ := row["id"].(float64)
		for _, f := range fields {
			v := strings.ToLower(fmt.Sprint(row[f]))
			if v == needle {
				exact = append(exact, int64(id))
				break
			}
			if strings.Contains(v, needle) {
				part = append(part, int64(id))
				break
			}
		}
	}
	m := part
	if len(exact) > 0 {
		m = exact
	}
	switch len(m) {
	case 0:
		return nil, &Err{404, fmt.Sprintf("%q not found in %s", raw, path)}
	case 1:
		return &m[0], nil
	}
	return nil, &Err{422, fmt.Sprintf("several matches for %q in %s; pass a numeric id", raw, path)}
}

func clampLimit(n, def int) int {
	if n <= 0 {
		return def
	}
	if n > 100 {
		return 100
	}
	return n
}

// getPage fetches a compact-list endpoint. A server from before the summary API ignores the
// parameters and answers with a plain array: say so instead of failing on the shape.
func getPage(ctx context.Context, c *Client, path string, q url.Values) (map[string]any, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, http.MethodGet, path, q, nil, &raw); err != nil {
		return nil, err
	}
	var page map[string]any
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, &Err{501, "the Quests server is older than this MCP (no compact lists): update the server"}
	}
	return page, nil
}
