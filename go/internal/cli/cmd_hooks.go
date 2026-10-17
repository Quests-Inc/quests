package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

func cmdHook(c *Client, args []string, asJSON bool) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("hook: list|show|add|remove|enable|disable|events")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdHookList(c, rest, asJSON)
	case "show":
		return cmdHookShow(c, rest, asJSON)
	case "add":
		return cmdHookAdd(c, rest, asJSON)
	case "remove", "rm", "delete":
		return cmdHookRemove(c, rest, asJSON)
	case "enable":
		return cmdHookEnable(c, rest, asJSON, true)
	case "disable":
		return cmdHookEnable(c, rest, asJSON, false)
	case "events":
		return cmdHookEvents(c, asJSON)
	default:
		return 1, fmt.Errorf("hook: неизвестно %s", sub)
	}
}

func cmdHookList(c *Client, args []string, asJSON bool) (int, error) {
	fs := flagSet("hook-list")
	quest := fs.Int64("quest", -1, "")
	globalOnly := fs.Bool("global", false, "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	raw, err := c.Get("/api/hooks", nil)
	if err != nil {
		return 1, err
	}
	var list []apiHook
	if err := json.Unmarshal(raw, &list); err != nil {
		return 1, err
	}
	filtered := list[:0]
	for _, h := range list {
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
		Emit(true, filtered, "")
		return 0, nil
	}
	if len(filtered) == 0 {
		fmt.Println("(хуков нет)")
		return 0, nil
	}
	for _, h := range filtered {
		fmt.Println(fmtAPIHookLine(h))
	}
	return 0, nil
}

func cmdHookShow(c *Client, args []string, asJSON bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	id, err := parseHookID(args[0])
	if err != nil {
		return 1, err
	}
	raw, err := c.Get(fmt.Sprintf("/api/hooks/%d", id), nil)
	if err != nil {
		return 1, err
	}
	var h apiHook
	if err := json.Unmarshal(raw, &h); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, h, "")
	} else {
		fmt.Println(fmtAPIHookLine(h))
		fmt.Println("  events:        ", strings.Join(h.Events, ", "))
		fmt.Println("  script_id:      ", h.ScriptID)
		fmt.Println("  timeout_sec:    ", h.TimeoutSec)
	}
	return 0, nil
}

func cmdHookAdd(c *Client, args []string, asJSON bool) (int, error) {
	fs := flagSet("hook-add")
	var events multiFlag
	fs.Var(&events, "event", "")
	fs.Var(&events, "e", "")
	script := fs.Int64("script", 0, "")
	fs.Int64Var(script, "script-id", 0, "")
	quest := fs.Int64("quest", -1, "")
	name := fs.String("name", "", "")
	runOn := fs.String("run-on", "", "")
	timeout := fs.Int("timeout", 30, "")
	disabled := fs.Bool("disabled", false, "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	if len(events) == 0 || *script < 1 {
		return 1, fmt.Errorf("нужны --event и --script (id скрипта из библиотеки)")
	}
	body := map[string]any{
		"name":        strings.TrimSpace(*name),
		"events":      []string(events),
		"script_id":   *script,
		"run_on":      strings.TrimSpace(*runOn),
		"timeout_sec": *timeout,
		"enabled":     !*disabled,
	}
	if *quest >= 0 {
		body["quest_id"] = *quest
	}
	raw, err := c.Post("/api/hooks", nil, body)
	if err != nil {
		return 1, err
	}
	var h apiHook
	if err := json.Unmarshal(raw, &h); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, h, "")
	} else {
		scope := "workspace"
		if h.QuestID != nil {
			scope = fmt.Sprintf("quest #%d", *h.QuestID)
		}
		fmt.Printf("хук #%d добавлен (%s, script %d)\n", h.ID, scope, h.ScriptID)
		fmt.Println(fmtAPIHookLine(h))
	}
	return 0, nil
}

func cmdHookRemove(c *Client, args []string, asJSON bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	id, err := parseHookID(args[0])
	if err != nil {
		return 1, err
	}
	if _, err := c.Delete(fmt.Sprintf("/api/hooks/%d", id), nil); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, map[string]any{"ok": true, "removed": id}, "")
	} else {
		fmt.Printf("удалён хук #%d\n", id)
	}
	return 0, nil
}

func cmdHookEnable(c *Client, args []string, asJSON bool, enabled bool) (int, error) {
	if len(args) < 1 {
		return 1, fmt.Errorf("нужен hook_id")
	}
	id, err := parseHookID(args[0])
	if err != nil {
		return 1, err
	}
	raw, err := c.Patch(fmt.Sprintf("/api/hooks/%d", id), nil, map[string]any{"enabled": enabled})
	if err != nil {
		return 1, err
	}
	var h apiHook
	if err := json.Unmarshal(raw, &h); err != nil {
		return 1, err
	}
	if asJSON {
		Emit(true, h, "")
	} else if enabled {
		fmt.Printf("хук #%d включён\n", h.ID)
	} else {
		fmt.Printf("хук #%d выключен\n", h.ID)
	}
	return 0, nil
}

func cmdHookEvents(c *Client, asJSON bool) (int, error) {
	raw, err := c.Get("/api/hook-events", nil)
	if err != nil {
		// Offline fallback: the same aliases the server ships.
		table := HookEventsTable()
		if asJSON {
			Emit(true, map[string]any{"aliases": aliasesFromTable(table), "kinds": []string{}}, "")
			return 0, nil
		}
		for _, row := range table {
			kinds, _ := json.Marshal(row["kinds"])
			fmt.Printf("%-18s → %s\n", row["alias"], string(kinds))
		}
		return 0, nil
	}
	if asJSON {
		Emit(true, json.RawMessage(raw), "")
		return 0, nil
	}
	var cat struct {
		Aliases map[string][]string `json:"aliases"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		return 1, err
	}
	for alias, kinds := range cat.Aliases {
		b, _ := json.Marshal(kinds)
		fmt.Printf("%-18s → %s\n", alias, string(b))
	}
	return 0, nil
}

func aliasesFromTable(table []map[string]any) map[string][]string {
	out := map[string][]string{}
	for _, row := range table {
		alias, _ := row["alias"].(string)
		kinds, _ := row["kinds"].([]string)
		out[alias] = kinds
	}
	return out
}
