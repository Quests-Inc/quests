package cli

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

func cmdNote(c *Client, args []string) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("note: list|show|add|set|delete")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdNoteList(c, rest)
	case "show", "get":
		return cmdNoteShow(c, rest)
	case "add", "create", "new":
		return cmdNoteAdd(c, rest)
	case "set":
		return cmdNoteSet(c, rest)
	case "delete", "rm":
		return cmdNoteDelete(c, rest)
	default:
		return 1, fmt.Errorf("note: неизвестно %s", sub)
	}
}

func cmdNoteList(c *Client, args []string) (int, error) {
	fs := flagSet("note-list")
	parent := fs.String("parent", "", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	q := map[string]string{}
	if *parent != "" {
		if isNone(*parent) {
			q["parent_id"] = "null"
		} else {
			id, err := strconv.ParseInt(*parent, 10, 64)
			if err != nil {
				return 1, fmt.Errorf("некорректный parent")
			}
			q["parent_id"] = strconv.FormatInt(id, 10)
		}
	}
	raw, err := c.Get("/api/notes", q)
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
		fmt.Println(FmtNoteLine(it))
	}
	return 0, nil
}

func cmdNoteShow(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "note_id")
	if err != nil {
		return 1, err
	}
	raw, err := c.Get(fmt.Sprintf("/api/notes/%d", id), nil)
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
		fmt.Println(FmtNoteDetail(m))
	}
	return 0, nil
}

func cmdNoteAdd(c *Client, args []string) (int, error) {
	fs := flagSet("note-add")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	parent := fs.String("parent", "", "")
	pin := fs.Bool("pin", false, "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return 1, fmt.Errorf("нужен TITLE")
	}
	body := map[string]any{"title": strings.Join(pos, " "), "description": *desc, "pinned": *pin}
	if *parent != "" && !isNone(*parent) {
		pid, err := strconv.ParseInt(*parent, 10, 64)
		if err != nil {
			return 1, fmt.Errorf("некорректный parent")
		}
		body["parent_id"] = pid
	}
	raw, err := c.Post("/api/notes", nil, body)
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
		fmt.Printf("создана заметка #%d: %s\n", id, AsString(m["title"]))
	}
	return 0, nil
}

func cmdNoteSet(c *Client, args []string) (int, error) {
	id, rest, err := parseID(args, "note_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("note-set")
	title := fs.String("title", "", "")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	parent := fs.String("parent", "", "")
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
	if set["parent"] {
		if isNone(*parent) {
			body["parent_id"] = nil
		} else {
			pid, err := strconv.ParseInt(*parent, 10, 64)
			if err != nil {
				return 1, fmt.Errorf("некорректный parent")
			}
			body["parent_id"] = pid
		}
	}
	if len(body) == 0 {
		return 1, fmt.Errorf("нечего менять")
	}
	raw, err := c.Patch(fmt.Sprintf("/api/notes/%d", id), nil, body)
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
		fmt.Println(FmtNoteDetail(m))
	}
	return 0, nil
}

func cmdNoteDelete(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "note_id")
	if err != nil {
		return 1, err
	}
	_, err = c.Delete(fmt.Sprintf("/api/notes/%d", id), nil)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, map[string]any{"ok": true, "deleted": id}, "")
	} else {
		fmt.Printf("удалена заметка #%d\n", id)
	}
	return 0, nil
}
