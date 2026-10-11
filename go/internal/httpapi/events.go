package httpapi

import (
	"encoding/json"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/valirum/quests/go/internal/platform/events"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const wsPingInterval = 25 * time.Second

func (s *Server) getSync(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int{"revision": s.Hub.GetRevision()})
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	since := 0
	if raw := r.URL.Query().Get("since"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			since = n
		}
	}
	ev := s.Hub.EventsSince(since, wsctx.From(r.Context()))
	if ev == nil {
		ev = []events.Payload{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"revision": s.Hub.GetRevision(),
		"events":   ev,
	})
}

func (s *Server) postFocusQuest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QuestID int64 `json:"quest_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuestID < 1 {
		writeErr(w, http.StatusBadRequest, "quest_id required")
		return
	}
	clients := s.Hub.FocusQuest(body.QuestID, wsctx.From(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{
		"quest_id":      body.QuestID,
		"clients":       clients,
		"pending_focus": s.Hub.PeekPendingFocus(wsctx.From(r.Context())),
	})
}

func (s *Server) wsEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	client, hello := s.Hub.AddClient(conn, wsctx.From(r.Context()))
	if err := client.WriteJSON(hello); err != nil {
		s.Hub.RemoveClient(client)
		return
	}

	done := make(chan struct{})
	defer func() {
		close(done)
		s.Hub.RemoveClient(client)
	}()

	// App-level JSON ping (frontend answers with text "pong"). Do not use
	// SetReadDeadline for keep-alive: gorilla stores timeout in readErr
	// permanently, and retrying ReadMessage spins until panic.
	go func() {
		t := time.NewTicker(wsPingInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if err := client.WriteJSON(map[string]string{"type": "ping"}); err != nil {
					_ = client.Conn.Close()
					return
				}
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) publishOpts(r *http.Request) (quiet bool, source string) {
	quiet = events.QuietFromQuery(r.URL.Query().Get("quiet"))
	source = r.URL.Query().Get("source")
	if source == "" {
		// Clients label themselves in X-Quests-Source (web, mcp, cli…); the web
		// UI tells its own edits apart from everyone else's by it.
		if h := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Quests-Source"))); sourcePattern.MatchString(h) {
			source = h
		}
	}
	source = events.SourceLabel(quiet, source)
	return quiet, source
}
