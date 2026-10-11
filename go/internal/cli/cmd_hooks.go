package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func cmdHook(args []string, asJSON bool) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("hook: list|show|add|remove|enable|disable|events")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdHookList(rest, asJSON)
	case "show":
		return cmdHookShow(rest, asJSON)
	case "add":
		return cmdHookAdd(rest, asJSON)
	case "remove", "rm", "delete":
		return cmdHookRemove(rest, asJSON)
	case "enable":
		return cmdHookEnable(rest, asJSON, true)
	case "disable":
		return cmdHookEnable(rest, asJSON, false)
	case "events":
		return cmdHookEvents(asJSON)
	default:
		return 1, fmt.Errorf("hook: неизвестно %s", sub)
	}
}

func cmdHookList(args []string, asJSON bool) (int, error) {
	fs := flagSet("hook-list")
	quest := fs.Int64("quest", -1, "")
	globalOnly := fs.Bool("global", false, "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	hooks, err := LoadHooks()
	if err != nil {
		return 1, err
	}
	filtered := hooks[:0]
	for _, h := range hooks {
		if *quest >= 0 {
			if h.QuestID != nil && *h.QuestID == *quest {
				filtered = append(filtered, h)
			}
			continue
		}
		if *globalOnly {
			if h.QuestID == nil {
				filtered = append(filtered, h)
			}
			continue
		}
		filtered = append(filtered, h)
	}
	if asJSON {
		arr := make([]map[string]any, 0, len(filtered))
		for _, h := range filtered {
			arr = append(arr, hookDict(h))
		}
		Emit(true, arr, "")
		return 0, nil
	}
	if len(filtered) == 0 {
		fmt.Println("(хуков нет)")
		return 0, nil
	}
	for _, h := range filtered {
		fmt.Println(FmtHookLine(h))
	}
	return 0, nil
}

func hookDict(h Hook) map[string]any {
	return map[string]any{
		"id": h.ID, "name": h.Name, "enabled": h.Enabled,
		"events": h.EventsRaw, "events_expanded": h.Events, "type": h.Type,
		"quest_id": h.QuestID, "command": h.Command, "url": h.URL, "path": h.Path,
		"timeout_sec": h.TimeoutSec, "workspace": h.Workspace,
	}
}

func cmdHookShow(args []string, asJSON bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	h, _, err := FindHook(args[0])
	if err != nil {
		return 1, err
	}
	if h == nil {
		return 1, fmt.Errorf("хук %q не найден", args[0])
	}
	if asJSON {
		Emit(true, hookDict(*h), "")
	} else {
		fmt.Println(FmtHookLine(*h))
		fmt.Println("  events_expanded:", strings.Join(h.Events, ", "))
		fmt.Println("  timeout_sec:    ", h.TimeoutSec)
		fmt.Println("  file:           ", hooksPath())
	}
	return 0, nil
}

func cmdHookAdd(args []string, asJSON bool) (int, error) {
	fs := flagSet("hook-add")
	var events multiFlag
	fs.Var(&events, "event", "")
	fs.Var(&events, "e", "")
	typ := fs.String("type", "", "")
	fs.StringVar(typ, "t", "", "")
	quest := fs.Int64("quest", -1, "")
	name := fs.String("name", "", "")
	command := fs.String("command", "", "")
	fs.StringVar(command, "c", "", "")
	url := fs.String("url", "", "")
	path := fs.String("path", "", "")
	timeout := fs.Float64("timeout", 30, "")
	disabled := fs.Bool("disabled", false, "")
	workspace := fs.String("workspace", "1", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	if len(events) == 0 || *typ == "" {
		return 1, fmt.Errorf("нужны --event и --type")
	}
	var qid *int64
	if *quest >= 0 {
		qid = quest
	}
	var ws int64
	switch strings.ToLower(strings.TrimSpace(*workspace)) {
	case "all", "*", "0":
		ws = 0 // operator-wide: events of every workspace
	default:
		n, perr := strconv.ParseInt(strings.TrimSpace(*workspace), 10, 64)
		if perr != nil || n < 1 {
			return 1, fmt.Errorf("--workspace: номер пространства или all")
		}
		ws = n
	}
	h, err := AddHook(events, *typ, qid, *name, *command, *url, *path, *timeout, !*disabled, ws)
	if err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, hookDict(h), "")
	} else {
		scope := "global"
		if h.QuestID != nil {
			scope = fmt.Sprintf("quest #%d", *h.QuestID)
		}
		fmt.Printf("хук %s добавлен (%s, %s)\n", h.ID, scope, h.Type)
		fmt.Println(FmtHookLine(h))
	}
	return 0, nil
}

func cmdHookRemove(args []string, asJSON bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	h, hooks, err := FindHook(args[0])
	if err != nil {
		return 1, err
	}
	if h == nil {
		return 1, fmt.Errorf("хук %q не найден", args[0])
	}
	keep := make([]Hook, 0, len(hooks)-1)
	for _, x := range hooks {
		if x.ID != h.ID {
			keep = append(keep, x)
		}
	}
	if err := SaveHooks(keep); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, map[string]any{"ok": true, "removed": hookDict(*h)}, "")
	} else {
		fmt.Printf("удалён хук %s\n", h.ID)
	}
	return 0, nil
}

func cmdHookEnable(args []string, asJSON bool, enabled bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	h, hooks, err := FindHook(args[0])
	if err != nil {
		return 1, err
	}
	if h == nil {
		return 1, fmt.Errorf("хук %q не найден", args[0])
	}
	for i := range hooks {
		if hooks[i].ID == h.ID {
			hooks[i].Enabled = enabled
			h = &hooks[i]
			break
		}
	}
	if err := SaveHooks(hooks); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, hookDict(*h), "")
	} else if enabled {
		fmt.Printf("хук %s включён\n", h.ID)
	} else {
		fmt.Printf("хук %s выключен\n", h.ID)
	}
	return 0, nil
}

func cmdHookEvents(asJSON bool) (int, error) {
	table := HookEventsTable()
	if asJSON {
		Emit(true, table, "")
		return 0, nil
	}
	for _, row := range table {
		kinds, _ := json.Marshal(row["kinds"])
		fmt.Printf("%-18s → %s\n", row["alias"], string(kinds))
	}
	return 0, nil
}
