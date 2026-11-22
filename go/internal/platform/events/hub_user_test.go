package events

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// A personal event reaches every socket of its author and nobody else in the workspace, and it does not wake hooks.
func TestPersonalEventReachesOnlyThatUser(t *testing.T) {
	h := New()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		user, _ := strconv.ParseInt(r.URL.Query().Get("user"), 10, 64)
		client, hello := h.AddClient(conn, 1, user)
		if err := client.WriteJSON(hello); err != nil {
			h.RemoveClient(client)
			return
		}
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					h.RemoveClient(client)
					return
				}
			}
		}()
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dial := func(user string) *websocket.Conn {
		t.Helper()
		c, _, err := websocket.DefaultDialer.Dial(url+"?user="+user, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		var hello map[string]any
		if err := c.ReadJSON(&hello); err != nil {
			t.Fatal(err)
		}
		return c
	}
	alice, again, bob := dial("7"), dial("7"), dial("8")

	hooked := make(chan struct{}, 1)
	h.OnPublish(func(Payload) { hooked <- struct{}{} })
	silent := ""
	h.Publish("graph_layout_created", PublishOpts{
		Workspace: 1, User: 7, Personal: true, Title: "Сессия 2", Toast: false, Sound: &silent,
		Extra: map[string]any{"layout_id": int64(5)},
	})

	read := func(c *websocket.Conn) map[string]any {
		t.Helper()
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		var msg map[string]any
		if err := c.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		return msg
	}
	for _, c := range []*websocket.Conn{alice, again} {
		msg := read(c)
		if msg["kind"] != "graph_layout_created" || msg["title"] != "Сессия 2" {
			t.Fatalf("author: %v", msg)
		}
		if _, ok := msg["state"].(map[string]any); ok {
			t.Fatalf("state leaked: %v", msg)
		}
	}
	_ = bob.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var msg map[string]any
	if err := bob.ReadJSON(&msg); err == nil {
		t.Fatalf("another user received %v", msg)
	}
	select {
	case <-hooked:
		t.Fatal("a personal layout woke hooks")
	case <-time.After(50 * time.Millisecond):
	}
	if ev := h.EventsSince(0, 1); len(ev) != 0 {
		t.Fatalf("workspace poll saw it: %v", ev)
	}
	if ev := h.EventsFor(0, 1, 7); len(ev) != 1 || ev[0]["title"] != "Сессия 2" {
		t.Fatalf("author poll: %v", ev)
	}
	if ev := h.EventsFor(0, 1, 8); len(ev) != 0 {
		t.Fatalf("other poll: %v", ev)
	}
}
