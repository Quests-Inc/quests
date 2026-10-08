package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Dispatch runs matching hooks for an event payload (kind + quest_id + …).
// Skips startup. Errors are logged; does not fail the publisher.
func Dispatch(event map[string]any) int {
	kind, _ := event["kind"].(string)
	if kind == "" || kind == "startup" {
		return 0
	}
	var qid *int64
	switch v := event["quest_id"].(type) {
	case float64:
		i := int64(v)
		qid = &i
	case int64:
		qid = &v
	case int:
		i := int64(v)
		qid = &i
	case json.Number:
		if n, err := v.Int64(); err == nil {
			qid = &n
		}
	}
	ws := eventWorkspace(event)
	list, err := Load("")
	if err != nil {
		log.Printf("hooks load: %v", err)
		return 0
	}
	n := 0
	for _, h := range list {
		if !h.Matches(kind, qid, ws) {
			continue
		}
		n++
		run(h, event)
	}
	return n
}

func run(h Hook, event map[string]any) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	timeout := time.Duration(h.TimeoutSec * float64(time.Second))
	if timeout < time.Second {
		timeout = 30 * time.Second
	}
	switch h.Type {
	case "script":
		cmd := exec.Command("sh", "-c", h.Command)
		cmd.Env = append(os.Environ(),
			"QUESTS_KIND="+fmt.Sprint(event["kind"]),
			"QUESTS_QUEST_ID="+fmt.Sprint(nullStr(event["quest_id"])),
			"QUESTS_TITLE="+fmt.Sprint(event["title"]),
			"QUESTS_DETAIL="+fmt.Sprint(event["detail"]),
			"QUESTS_PAYLOAD="+string(payload),
		)
		cmd.Stdin = bytes.NewReader(append(payload, '\n'))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		done := make(chan error, 1)
		go func() { done <- cmd.Run() }()
		select {
		case err := <-done:
			if err != nil {
				log.Printf("hook %s script: %v", h.ID, err)
			}
		case <-time.After(timeout):
			_ = cmd.Process.Kill()
			log.Printf("hook %s script: timeout", h.ID)
		}
	case "webhook":
		client := &http.Client{Timeout: timeout}
		req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(payload))
		if err != nil {
			log.Printf("hook %s webhook: %v", h.ID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("hook %s webhook: %v", h.ID, err)
			return
		}
		_ = resp.Body.Close()
	case "socket":
		conn, err := net.DialTimeout("unix", h.Path, timeout)
		if err != nil {
			log.Printf("hook %s socket: %v", h.ID, err)
			return
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))
		_, err = conn.Write(append(payload, '\n'))
		_ = conn.Close()
		if err != nil {
			log.Printf("hook %s socket: %v", h.ID, err)
		}
	}
}

func nullStr(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// eventWorkspace reads the workspace the hub stamped on the event (0 = none).
func eventWorkspace(event map[string]any) int64 {
	switch v := event["workspace"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}
