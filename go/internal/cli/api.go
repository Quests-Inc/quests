package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	Base   string
	HTTP   *http.Client
	AsJSON bool
	// Token is the bearer credential for a instance with accounts enabled.
	// Defaults to QUESTS_API_TOKEN; empty against an open local instance.
	Token string
	// Source is sent as X-Quests-Source (how the quests this client creates are
	// labelled); "cli" by default.
	Source string
}

func NewClient(base string, asJSON bool) *Client {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = defaultAPI()
	}
	return &Client{
		Base:   base,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		AsJSON: asJSON,
		Token:  strings.TrimSpace(os.Getenv("QUESTS_API_TOKEN")),
		Source: "cli",
	}
}

func defaultAPI() string {
	if v := strings.TrimSpace(os.Getenv("QUESTS_API")); v != "" {
		return strings.TrimRight(v, "/")
	}
	host := strings.TrimSpace(os.Getenv("QUESTS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("QUESTS_PORT"))
	if port == "" {
		port = "8765"
	}
	return "http://" + host + ":" + port
}

type APIError struct {
	Code   int
	Detail string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API %d: %s", e.Code, e.Detail)
}

func (c *Client) Do(method, path string, query map[string]string, body any) (json.RawMessage, error) {
	u := c.Base + path
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			if v != "" {
				q.Set(k, v)
			}
		}
		if enc := q.Encode(); enc != "" {
			u += "?" + enc
		}
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Quests-Source", c.Source)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	// Pick a workspace explicitly (tokens are pinned, this only narrows/switches within membership).
	if ws := strings.TrimSpace(os.Getenv("QUESTS_WORKSPACE")); ws != "" {
		req.Header.Set("X-Quests-Workspace", ws)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось связаться с API (%s): %w. Запусти: QUESTS_PORT=8765 ./scripts/run-go-server.sh", c.Base, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		detail := string(raw)
		var parsed map[string]any
		if json.Unmarshal(raw, &parsed) == nil {
			if d, ok := parsed["detail"]; ok {
				detail = fmt.Sprint(d)
			}
		}
		return nil, &APIError{Code: resp.StatusCode, Detail: detail}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

func (c *Client) Get(path string, query map[string]string) (json.RawMessage, error) {
	return c.Do(http.MethodGet, path, query, nil)
}
func (c *Client) Post(path string, query map[string]string, body any) (json.RawMessage, error) {
	return c.Do(http.MethodPost, path, query, body)
}
func (c *Client) Patch(path string, query map[string]string, body any) (json.RawMessage, error) {
	return c.Do(http.MethodPatch, path, query, body)
}
func (c *Client) Delete(path string, query map[string]string) (json.RawMessage, error) {
	return c.Do(http.MethodDelete, path, query, nil)
}

func Emit(asJSON bool, data any, text string) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(data)
		return
	}
	if text != "" {
		fmt.Println(text)
	}
}

func EmitError(asJSON bool, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": msg})
	} else {
		fmt.Fprintln(os.Stderr, "error:", msg)
	}
	return 1
}

func DecodeList(raw json.RawMessage) ([]map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func DecodeMap(raw json.RawMessage) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func AsInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func AsString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func AsBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	default:
		return false
	}
}
