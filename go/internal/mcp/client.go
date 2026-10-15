package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// WorkspaceHeader names the workspace a request acts on (a user's token may address any workspace they belong to).
const WorkspaceHeader = "X-Quests-Workspace"

// Workspace is one workspace of the account, as GET /api/workspaces lists it.
type Workspace struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

// Client talks to the Quests HTTP API as one token, acting in the active workspace.
type Client struct {
	base  string
	token string
	hc    *http.Client

	mu         sync.RWMutex
	loaded     bool
	workspaces []Workspace
	active     *Workspace // nil: the token's own workspace (no header sent)
}

func NewClient(base, token string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"), token: strings.TrimSpace(token),
		hc: &http.Client{Timeout: 30 * time.Second},
	}
}

// Err is an API failure in the form tools report it: "<code>: <what is wrong>".
type Err struct {
	Code int
	Msg  string
}

func (e *Err) Error() string {
	s := fmt.Sprintf("%d: %s", e.Code, e.Msg)
	switch e.Code {
	case http.StatusForbidden:
		s += " (check list_workspaces / switch_workspace)"
	case http.StatusNotFound:
		s += " (wrong id, or the item is in another workspace: see list_workspaces)"
	}
	return s
}

// loadWorkspaces fills the list once and makes the first workspace active.
// A token that cannot list workspaces (auth off, a workspace-less token) simply works without the header.
func (c *Client) loadWorkspaces(ctx context.Context) {
	c.mu.RLock()
	done := c.loaded
	c.mu.RUnlock()
	if done {
		return
	}
	var view struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	err := c.do(ctx, http.MethodGet, "/api/workspaces", nil, nil, &view, false)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	if err == nil {
		c.workspaces = view.Workspaces
		if len(c.workspaces) > 0 {
			w := c.workspaces[0]
			c.active = &w
		}
	}
	c.loaded = true
}

// Workspaces returns the account's workspaces and the active one's name (empty if the token has none).
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, string) {
	c.loadWorkspaces(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Workspace(nil), c.workspaces...), c.activeNameLocked()
}

// Workspace is the active workspace's name for tool results.
func (c *Client) Workspace(ctx context.Context) string {
	c.loadWorkspaces(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeNameLocked()
}

func (c *Client) activeNameLocked() string {
	if c.active == nil {
		return ""
	}
	return c.active.Name
}

// Switch makes the workspace with that name (or numeric id) active and returns the previous and the new name.
func (c *Client) Switch(ctx context.Context, name string) (prev, cur string, err error) {
	c.loadWorkspaces(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	prev = c.activeNameLocked()
	needle := strings.ToLower(strings.TrimSpace(name))
	var found *Workspace
	for i := range c.workspaces {
		w := c.workspaces[i]
		if strings.ToLower(w.Name) == needle || fmt.Sprint(w.ID) == needle {
			found = &w
			break
		}
	}
	if found == nil {
		names := make([]string, 0, len(c.workspaces))
		for _, w := range c.workspaces {
			names = append(names, w.Name)
		}
		return prev, prev, &Err{http.StatusNotFound, fmt.Sprintf("unknown workspace %q; known: %s", name, strings.Join(names, ", "))}
	}
	c.active = found
	return prev, found.Name, nil
}

// Do sends a JSON request in the active workspace and decodes the JSON answer into out (nil = ignore it).
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	c.loadWorkspaces(ctx)
	return c.do(ctx, method, path, query, body, out, true)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any, scoped bool) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	c.sign(req, scoped)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	data, err := c.send(req)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Raw fetches a path and returns the body and its Content-Type (attachments, icons).
func (c *Client) Raw(ctx context.Context, path string, query url.Values) ([]byte, string, error) {
	c.loadWorkspaces(ctx)
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	c.sign(req, true)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("cannot reach Quests API (%s): %w", c.base, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, "", apiErr(resp.StatusCode, data)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// Upload posts a multipart body (content type from the writer) in the active workspace.
func (c *Client) Upload(ctx context.Context, method, path, contentType string, body io.Reader, out any) error {
	c.loadWorkspaces(ctx)
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	c.sign(req, true)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	data, err := c.send(req)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) sign(req *http.Request, scoped bool) {
	req.Header.Set("X-Quests-Source", "mcp")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if !scoped {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.active != nil {
		req.Header.Set(WorkspaceHeader, fmt.Sprint(c.active.ID))
	}
}

func (c *Client) send(req *http.Request) ([]byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Quests API (%s): %w", c.base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, apiErr(resp.StatusCode, data)
	}
	return data, nil
}

func apiErr(code int, body []byte) error {
	var d struct {
		Detail any `json:"detail"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &d) == nil && d.Detail != nil {
		if s, ok := d.Detail.(string); ok {
			msg = s
		} else if b, err := json.Marshal(d.Detail); err == nil {
			msg = string(b)
		}
	}
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}
	if msg == "" {
		msg = http.StatusText(code)
	}
	return &Err{Code: code, Msg: msg}
}
