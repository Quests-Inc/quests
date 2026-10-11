package cli

import (
	"flag"
	"fmt"
	"strings"
)

func cmdCategories(c *Client, _ []string) (int, error) {
	raw, err := c.Get("/api/categories", nil)
	if err != nil {
		return 1, err
	}
	items, err := DecodeList(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, items, "")
		return 0, nil
	}
	if len(items) == 0 {
		fmt.Println("(пусто)")
		return 0, nil
	}
	for _, it := range items {
		fmt.Println(FmtCategoryLine(it))
	}
	return 0, nil
}

func cmdQuestline(c *Client, args []string) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("questline: list|show|add|set|delete")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdQLList(c, rest)
	case "show", "get":
		return cmdQLShow(c, rest)
	case "add", "create", "new":
		return cmdQLAdd(c, rest)
	case "set":
		return cmdQLSet(c, rest)
	case "delete", "rm":
		return cmdQLDelete(c, rest)
	default:
		return 1, fmt.Errorf("questline: неизвестно %s", sub)
	}
}

func cmdQLList(c *Client, args []string) (int, error) {
	fs := flagSet("ql-list")
	category := fs.String("category", "", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	raw, err := c.Get("/api/questlines", nil)
	if err != nil {
		return 1, err
	}
	items, err := DecodeList(raw)
	if err != nil {
		return 1, err
	}
	if *category != "" {
		cid, err := c.ResolveCategoryID(*category)
		if err != nil {
			return 1, err
		}
		filtered := items[:0]
		for _, it := range items {
			id, ok := AsInt64(it["category_id"])
			if cid == nil {
				if it["category_id"] == nil {
					filtered = append(filtered, it)
				}
			} else if ok && id == *cid {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if c.AsJSON {
		Emit(true, items, "")
		return 0, nil
	}
	if len(items) == 0 {
		fmt.Println("(пусто)")
		return 0, nil
	}
	for _, it := range items {
		fmt.Println(FmtQuestlineLine(it))
	}
	return 0, nil
}

func cmdQLShow(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "line_id")
	if err != nil {
		return 1, err
	}
	raw, err := c.Get(fmt.Sprintf("/api/questlines/%d", id), nil)
	if err != nil {
		return 1, err
	}
	m, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, m, "")
	} else {
		fmt.Println(FmtQuestlineDetail(m))
	}
	return 0, nil
}

func cmdQLAdd(c *Client, args []string) (int, error) {
	fs := flagSet("ql-add")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	category := fs.String("category", "", "")
	color := fs.String("color", "#9a9a9a", "")
	icon := fs.String("icon", "document", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return 1, fmt.Errorf("нужен TITLE")
	}
	body := map[string]any{"title": strings.Join(pos, " "), "description": *desc, "color": *color, "icon": *icon}
	if *category != "" {
		cid, err := c.ResolveCategoryID(*category)
		if err != nil {
			return 1, err
		}
		body["category_id"] = cid
	}
	raw, err := c.Post("/api/questlines", nil, body)
	if err != nil {
		return 1, err
	}
	m, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, m, "")
	} else {
		id, _ := AsInt64(m["id"])
		fmt.Printf("создан квестлайн #%d: %s\n", id, AsString(m["title"]))
	}
	return 0, nil
}

func cmdQLSet(c *Client, args []string) (int, error) {
	id, rest, err := parseID(args, "line_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("ql-set")
	title := fs.String("title", "", "")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	category := fs.String("category", "", "")
	color := fs.String("color", "", "")
	icon := fs.String("icon", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2, nil
	}
	body := map[string]any{}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["title"] {
		body["title"] = *title
	}
	if set["d"] || set["description"] {
		body["description"] = *desc
	}
	if set["category"] {
		cid, err := c.ResolveCategoryID(*category)
		if err != nil {
			return 1, err
		}
		body["category_id"] = cid
	}
	if set["color"] {
		body["color"] = *color
	}
	if set["icon"] {
		body["icon"] = *icon
	}
	if len(body) == 0 {
		return 1, fmt.Errorf("нечего менять")
	}
	raw, err := c.Patch(fmt.Sprintf("/api/questlines/%d", id), nil, body)
	if err != nil {
		return 1, err
	}
	m, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, m, "")
	} else {
		fmt.Println(FmtQuestlineDetail(m))
	}
	return 0, nil
}

func cmdQLDelete(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "line_id")
	if err != nil {
		return 1, err
	}
	_, err = c.Delete(fmt.Sprintf("/api/questlines/%d", id), nil)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, map[string]any{"ok": true, "deleted": id}, "")
	} else {
		fmt.Printf("удалён квестлайн #%d\n", id)
	}
	return 0, nil
}
