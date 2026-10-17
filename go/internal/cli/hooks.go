package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/hooks"
)

// apiHook is a workspace hook from GET /api/hooks.
type apiHook struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Events      []string `json:"events"`
	EventsRaw   []string `json:"events_raw"`
	ScriptID    int64    `json:"script_id"`
	ScriptTitle string   `json:"script_title"`
	QuestID     *int64   `json:"quest_id"`
	RunOn       string   `json:"run_on"`
	TimeoutSec  int      `json:"timeout_sec"`
}

func HookEventsTable() []map[string]any {
	out := make([]map[string]any, 0)
	for alias, kinds := range hooks.EventAliases() {
		out = append(out, map[string]any{"alias": alias, "kinds": kinds})
	}
	return out
}

func fmtAPIHookLine(h apiHook) string {
	en := "off"
	if h.Enabled {
		en = "on"
	}
	ev := h.EventsRaw
	if len(ev) == 0 {
		ev = h.Events
	}
	scope := "workspace"
	if h.QuestID != nil {
		scope = fmt.Sprintf("quest #%d", *h.QuestID)
	}
	script := fmt.Sprintf("script %d", h.ScriptID)
	if h.ScriptTitle != "" {
		script = h.ScriptTitle
	}
	name := h.Name
	if name == "" {
		name = "—"
	}
	return fmt.Sprintf("#%d [%s] %s · %s · %s · %s · run_on=%s",
		h.ID, en, name, scope, strings.Join(ev, ","), script, h.RunOn)
}

func parseHookID(s string) (int64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("нужен числовой id хука")
	}
	return id, nil
}
