package events

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const FocusTTL = 45 * time.Second
const HistorySize = 64
const writeWait = 5 * time.Second

// Payload is a domain event broadcast to WS / polling clients.
type Payload map[string]any

// Client is one WS peer. All writes go through WriteJSON / WriteRaw under mu
// (gorilla allows one concurrent writer).
type Client struct {
	Conn *websocket.Conn
	// Workspace is the workspace this peer listens to (0 = all, i.e. no workspaces).
	Workspace int64
	mu        sync.Mutex
}

// systemWide marks an event meant for every peer (server start). An event with no workspace
// that is NOT system-wide is a caller's omission: it reaches only peers that listen to
// everything (no workspaces), never another workspace's.
const systemWide int64 = -1

// visible reports whether an event of workspace ws is for this peer.
func (c *Client) visible(ws int64) bool {
	return ws == systemWide || c.Workspace == 0 || c.Workspace == ws
}

func (c *Client) WriteJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.Conn.WriteJSON(v)
}

func (c *Client) WriteRaw(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.Conn.WriteMessage(messageType, data)
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.Conn.Close()
}

type Hub struct {
	mu             sync.Mutex
	Revision       int
	clients        map[*Client]struct{}
	recent         []Payload
	pendingFocus   *int64
	pendingFocusAt time.Time
	pendingFocusWS int64
	observers      []func(Payload)
}

// OnPublish registers fn to be called, in its own goroutine, with a copy of every published event
// (the server wires user hooks this way, so that events does not know about them).
func (h *Hub) OnPublish(fn func(Payload)) {
	h.mu.Lock()
	h.observers = append(h.observers, fn)
	h.mu.Unlock()
}

func New() *Hub {
	return &Hub{
		clients: map[*Client]struct{}{},
		recent:  make([]Payload, 0, HistorySize),
	}
}

func (h *Hub) AddClient(conn *websocket.Conn, ws int64) (*Client, Payload) {
	c := &Client{Conn: conn, Workspace: ws}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	hello := Payload{"type": "hello", "revision": h.Revision}
	if focus := h.focusIfFreshLocked(ws); focus != nil {
		hello["pending_focus"] = *focus
	}
	return c, hello
}

func (h *Hub) RemoveClient(c *Client) {
	if c == nil {
		return
	}
	h.mu.Lock()
	_, ok := h.clients[c]
	if ok {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	if ok {
		c.Close()
	}
}

func (h *Hub) focusIfFreshLocked(ws int64) *int64 {
	if h.pendingFocus == nil || (ws != 0 && h.pendingFocusWS != 0 && ws != h.pendingFocusWS) {
		return nil
	}
	if time.Since(h.pendingFocusAt) > FocusTTL {
		return nil
	}
	return h.pendingFocus
}

func (h *Hub) PeekPendingFocus(ws int64) *int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.focusIfFreshLocked(ws)
}

func (h *Hub) SetPendingFocus(questID, ws int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingFocus = &questID
	h.pendingFocusAt = time.Now()
	h.pendingFocusWS = ws
}

func (h *Hub) GetRevision() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Revision
}

// EventsSince returns recent events newer than revision since that are visible
// from workspace ws (0 = all).
func (h *Hub) EventsSince(since int, ws int64) []Payload {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Payload, 0)
	for _, e := range h.recent {
		rev := asInt(e["revision"])
		ew := int64(asInt(e["workspace"]))
		if rev > since && (ws == 0 || ew == systemWide || ew == ws) {
			out = append(out, e)
		}
	}
	return out
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (h *Hub) clientSnapshot() []*Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		out = append(out, c)
	}
	return out
}

// send writes outside h.mu so a slow socket cannot stall Publish / HTTP.
func (h *Hub) send(raw []byte, ws int64) int {
	clients := h.clientSnapshot()
	delivered := 0
	for _, c := range clients {
		if !c.visible(ws) {
			continue
		}
		if err := c.WriteRaw(websocket.TextMessage, raw); err != nil {
			h.RemoveClient(c)
			continue
		}
		delivered++
	}
	return delivered
}

func (h *Hub) Broadcast(payload Payload, ws int64) int {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return h.send(raw, ws)
}

func (h *Hub) FocusQuest(questID, ws int64) int {
	h.SetPendingFocus(questID, ws)
	return h.Broadcast(Payload{"type": "ui_focus_quest", "quest_id": questID}, ws)
}

func (h *Hub) Publish(kind string, opts PublishOpts) Payload {
	h.mu.Lock()
	h.Revision++
	payload := Payload{
		"type":     "quests_changed",
		"kind":     kind,
		"revision": h.Revision,
		"quest_id": opts.QuestID,
		"title":    opts.Title,
		"detail":   opts.Detail,
		"toast":    opts.Toast,
	}
	sound := opts.Sound
	if sound == nil {
		payload["sound"] = kind
	} else {
		payload["sound"] = *sound
	}
	if opts.Description != "" {
		payload["description"] = opts.Description
	}
	if opts.Significance != "" {
		payload["significance"] = opts.Significance
	}
	if opts.Source != "" {
		payload["source"] = opts.Source
	}
	if opts.StepTitle != "" {
		payload["step_title"] = opts.StepTitle
	}
	if opts.Automated {
		payload["automated"] = true
	}
	target := opts.Workspace
	if opts.System {
		target = systemWide
	}
	if target != 0 {
		payload["workspace"] = target
	}
	for k, v := range opts.Extra {
		payload[k] = v
	}
	h.recent = append(h.recent, payload)
	if len(h.recent) > HistorySize {
		h.recent = h.recent[len(h.recent)-HistorySize:]
	}
	raw, _ := json.Marshal(payload)
	observers := append([]func(Payload){}, h.observers...)
	h.mu.Unlock()
	h.send(raw, target)
	// Fire-and-forget observers (user hooks and the like): each gets its own copy.
	for _, fn := range observers {
		go func(fn func(Payload), p Payload) {
			cp := make(Payload, len(p))
			for k, v := range p {
				cp[k] = v
			}
			fn(cp)
		}(fn, payload)
	}
	return payload
}

type PublishOpts struct {
	// Workspace routes the event to that workspace's peers (0 = everyone).
	Workspace int64
	// System sends the event to every peer whatever its workspace (no private data!).
	System       bool
	QuestID      *int64
	Title        string
	Detail       string
	Description  string
	Sound        *string
	Toast        bool
	Source       string
	Significance string
	StepTitle    string
	Automated    bool
	Extra        map[string]any
}

func SourceLabel(quiet bool, source string) string {
	if source != "" {
		return source
	}
	if quiet {
		return "quiet"
	}
	return "api"
}

func QuietFromQuery(raw string) bool {
	switch raw {
	case "1", "true", "True", "yes", "on":
		return true
	default:
		return false
	}
}

// ClientCount is the number of connected WebSocket peers.
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
