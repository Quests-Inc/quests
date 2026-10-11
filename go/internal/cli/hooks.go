package cli

import (
	"github.com/quests-inc/quests/go/internal/platform/hooks"
)

type Hook = hooks.Hook

func hooksPath() string { return hooks.Path() }

func LoadHooks() ([]Hook, error) { return hooks.Load("") }

func SaveHooks(list []Hook) error { return hooks.Save(list, "") }

func AddHook(events []string, hookType string, questID *int64, name, command, url, path string, timeout float64, enabled bool, workspace int64) (Hook, error) {
	return hooks.Add(events, hookType, questID, name, command, url, path, timeout, enabled, workspace)
}

func FindHook(id string) (*Hook, []Hook, error) { return hooks.Find(id) }

func HookEventsTable() []map[string]any {
	out := make([]map[string]any, 0)
	for alias, kinds := range hooks.EventAliases() {
		out = append(out, map[string]any{"alias": alias, "kinds": kinds})
	}
	return out
}
