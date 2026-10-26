package hooks

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var eventAliases = map[string][]string{
	"complete":         {"quest_completed"},
	"on_complete":      {"quest_completed"},
	"step":             {"step_completed", "step_progress"},
	"on_step":          {"step_completed", "step_progress"},
	"status":           {"status_changed", "quest_completed", "quest_failed", "quest_expired"},
	"on_status_change": {"status_changed", "quest_completed", "quest_failed", "quest_expired"},
	"fail":             {"quest_failed"},
	"created":          {"quest_created"},
	"deleted":          {"quest_deleted"},
	"appear":           {"quest_appeared", "quest_created"},
	"start":            {"quest_started"},
	"window":           {"quest_started"},
	"expire":           {"quest_expired"},
	"note":             {"note_created", "note_updated", "note_deleted"},
	"notes":            {"note_created", "note_updated", "note_deleted"},
	"value":            {"values_changed"},
	"variable":         {"values_changed"},
	"variables":        {"values_changed"},
	"attachment":       {"attachment_added", "attachment_revision"},
}

// DefaultWorkspace is where a hook lives when its file entry names none (everything that
// existed before workspaces did).
const DefaultWorkspace int64 = 1

type Hook struct {
	// Workspace limits the hook to one workspace's events; 0 = every workspace (operator-wide).
	Workspace  int64
	ID         string
	Name       string
	Enabled    bool
	Events     []string
	EventsRaw  []string
	Type       string
	QuestID    *int64
	Command    string
	URL        string
	Path       string
	TimeoutSec float64
}

// Matches reports whether the hook fires for an event. eventWorkspace is the workspace the event
// belongs to; 0 (system events, or a server without workspaces) counts as the default workspace.
func (h Hook) Matches(kind string, questID *int64, eventWorkspace int64) bool {
	if !h.Enabled {
		return false
	}
	if eventWorkspace <= 0 {
		eventWorkspace = DefaultWorkspace
	}
	if h.Workspace != 0 && h.Workspace != eventWorkspace {
		return false
	}
	ok := false
	for _, e := range h.Events {
		if e == kind {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	if h.QuestID == nil {
		return true
	}
	return questID != nil && *h.QuestID == *questID
}

func Path() string {
	if v := strings.TrimSpace(os.Getenv("QUESTS_HOOKS")); v != "" {
		return v
	}
	root := strings.TrimSpace(os.Getenv("QUESTS_ROOT"))
	if root == "" {
		wd, _ := os.Getwd()
		root = wd
		for {
			if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				break
			}
			root = parent
		}
	}
	return filepath.Join(root, "data", "hooks.json")
}

func ExpandEvents(names []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, raw := range names {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		targets, ok := eventAliases[key]
		if !ok {
			targets = []string{key}
		}
		for _, kind := range targets {
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			out = append(out, kind)
		}
	}
	return out
}

func EventAliases() map[string][]string {
	out := make(map[string][]string, len(eventAliases))
	for k, v := range eventAliases {
		cp := append([]string{}, v...)
		out[k] = cp
	}
	return out
}

type fileHook struct {
	// Workspace: absent = the default workspace, 0 = all workspaces, N = that workspace.
	Workspace  *int64   `json:"workspace,omitempty"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	Events     []string `json:"events"`
	Type       string   `json:"type"`
	QuestID    *int64   `json:"quest_id"`
	Command    string   `json:"command"`
	URL        string   `json:"url"`
	Path       string   `json:"path"`
	TimeoutSec float64  `json:"timeout_sec"`
}

func Load(path string) ([]Hook, error) {
	if path == "" {
		path = Path()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var wrap struct {
		Hooks []fileHook `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		var list []fileHook
		if err2 := json.Unmarshal(raw, &list); err2 != nil {
			return nil, nil
		}
		wrap.Hooks = list
	}
	out := make([]Hook, 0, len(wrap.Hooks))
	for _, h := range wrap.Hooks {
		expanded := ExpandEvents(h.Events)
		if len(expanded) == 0 {
			continue
		}
		typ := h.Type
		if typ == "" {
			typ = "script"
		}
		ws := DefaultWorkspace
		if h.Workspace != nil {
			ws = *h.Workspace
		}
		out = append(out, Hook{
			Workspace: ws, ID: h.ID, Name: h.Name, Enabled: h.Enabled,
			Events: expanded, EventsRaw: append([]string{}, h.Events...),
			Type: typ, QuestID: h.QuestID, Command: h.Command,
			URL: h.URL, Path: h.Path, TimeoutSec: h.TimeoutSec,
		})
	}
	return out, nil
}

func Save(hooks []Hook, path string) error {
	if path == "" {
		path = Path()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := struct {
		Hooks []fileHook `json:"hooks"`
	}{Hooks: make([]fileHook, 0, len(hooks))}
	for _, h := range hooks {
		ev := h.EventsRaw
		if len(ev) == 0 {
			ev = h.Events
		}
		ws := h.Workspace
		payload.Hooks = append(payload.Hooks, fileHook{
			Workspace: &ws, ID: h.ID, Name: h.Name, Enabled: h.Enabled, Events: ev, Type: h.Type,
			QuestID: h.QuestID, Command: h.Command, URL: h.URL, Path: h.Path, TimeoutSec: h.TimeoutSec,
		})
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Add(events []string, hookType string, questID *int64, name, command, url, path string, timeout float64, enabled bool, workspace int64) (Hook, error) {
	expanded := ExpandEvents(events)
	if len(expanded) == 0 {
		return Hook{}, fmt.Errorf("нужен хотя бы один event")
	}
	switch hookType {
	case "script":
		if strings.TrimSpace(command) == "" {
			return Hook{}, fmt.Errorf("script-хук требует --command")
		}
	case "webhook":
		if strings.TrimSpace(url) == "" {
			return Hook{}, fmt.Errorf("webhook-хук требует --url")
		}
	case "socket":
		if strings.TrimSpace(path) == "" {
			return Hook{}, fmt.Errorf("socket-хук требует --path")
		}
	default:
		return Hook{}, fmt.Errorf("type must be script|webhook|socket")
	}
	if timeout <= 0 {
		timeout = 30
	}
	raw := make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e != "" {
			raw = append(raw, e)
		}
	}
	h := Hook{
		Workspace: workspace, ID: newID(), Name: strings.TrimSpace(name), Enabled: enabled,
		Events: expanded, EventsRaw: raw, Type: hookType, QuestID: questID,
		Command: command, URL: url, Path: path, TimeoutSec: timeout,
	}
	hooks, err := Load("")
	if err != nil {
		return Hook{}, err
	}
	hooks = append(hooks, h)
	if err := Save(hooks, ""); err != nil {
		return Hook{}, err
	}
	return h, nil
}

func Find(id string) (*Hook, []Hook, error) {
	hooks, err := Load("")
	if err != nil {
		return nil, nil, err
	}
	want := strings.TrimSpace(id)
	for i := range hooks {
		if hooks[i].ID == want || (hooks[i].Name != "" && hooks[i].Name == want) {
			return &hooks[i], hooks, nil
		}
	}
	return nil, hooks, nil
}
