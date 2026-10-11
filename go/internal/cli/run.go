package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func Run(argv []string) int {
	asJSON, apiURL, rest := peelGlobals(argv)
	if len(rest) == 0 || rest[0] == "-h" || rest[0] == "--help" {
		printHelp()
		return 0
	}
	cmd := rest[0]
	args := rest[1:]
	c := NewClient(apiURL, asJSON)

	var err error
	var code int
	switch cmd {
	case "list", "ls":
		code, err = cmdList(c, args)
	case "show", "get":
		code, err = cmdShow(c, args)
	case "add", "create", "new":
		code, err = cmdAdd(c, args)
	case "llm-add", "add-llm", "new-llm":
		code, err = cmdLLMAdd(c, args)
	case "set":
		code, err = cmdSet(c, args)
	case "pin":
		code, err = cmdPin(c, args, false)
	case "unpin":
		code, err = cmdPin(c, args, true)
	case "status":
		code, err = cmdStatus(c, args, "")
	case "complete":
		code, err = cmdStatus(c, args, "completed")
	case "fail":
		code, err = cmdStatus(c, args, "failed")
	case "step":
		code, err = cmdStep(c, args)
	case "progress":
		code, err = cmdProgress(c, args)
	case "step-add", "stepadd":
		code, err = cmdStepAdd(c, args)
	case "step-edit", "stepedit", "step-set":
		code, err = cmdStepEdit(c, args)
	case "step-rm", "step-delete", "steprm":
		code, err = cmdStepRm(c, args)
	case "delete", "rm":
		code, err = cmdDelete(c, args)
	case "categories", "cats", "category":
		code, err = cmdCategories(c, args)
	case "questline", "ql", "questlines":
		code, err = cmdQuestline(c, args)
	case "note", "notes":
		code, err = cmdNote(c, args)
	case "hook":
		code, err = cmdHook(args, asJSON)
	case "-v", "--version", "version":
		fmt.Println("quests 0.2.0 (go)")
		return 0
	default:
		return EmitError(asJSON, "неизвестная команда: "+cmd+" (см. quests --help)")
	}
	if err != nil {
		return EmitError(asJSON, err.Error())
	}
	return code
}

func peelGlobals(argv []string) (asJSON bool, api string, rest []string) {
	api = defaultAPI()
	rest = make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		tok := argv[i]
		switch {
		case tok == "--json":
			asJSON = true
		case tok == "--api" && i+1 < len(argv):
			i++
			api = strings.TrimRight(argv[i], "/")
		case strings.HasPrefix(tok, "--api="):
			api = strings.TrimRight(strings.TrimPrefix(tok, "--api="), "/")
		default:
			rest = append(rest, tok)
		}
	}
	return asJSON, api, rest
}

func printHelp() {
	fmt.Print(`quests — CLI журнала Quests (Go binary)

Usage:
  quests [--json] [--api URL] COMMAND …

Build: just build-cli  →  go/bin/quests
Run:   just quests …  |  go/bin/quests …

Commands:
  list|ls          список квестов
  show|get ID      детали
  add TITLE        создать
  llm-add TEXT…    текст → действия через ассистента сервера (-y без вопроса)
  set ID           поля квеста
  pin|unpin ID
  status ID STATUS
  complete|fail ID
  step ID          прогресс шага
  progress ID      закрыто/всего шагов квеста как JSON для check_command (poll)
  step-add|step-edit|step-rm
  delete|rm ID
  categories|cats
  questline|ql …   list|show|add|set|delete
  note|notes …     list|show|add|set|delete
  hook …           list|show|add|remove|enable|disable|events

Env: QUESTS_API, QUESTS_HOOKS, QUESTS_ROOT
Docs: docs/cli.md
`)
}

func parseID(args []string, name string) (int64, []string, error) {
	if len(args) < 1 {
		return 0, args, fmt.Errorf("нужен %s", name)
	}
	n, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, args, fmt.Errorf("некорректный %s: %s", name, args[0])
	}
	return n, args[1:], nil
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func cmdList(c *Client, args []string) (int, error) {
	fs := flagSet("list")
	status := fs.String("status", "", "")
	pinned := fs.Bool("pinned", false, "")
	unpinned := fs.Bool("unpinned", false, "")
	category := fs.String("category", "", "")
	questline := fs.String("questline", "", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	q := map[string]string{}
	if *status != "" {
		q["status"] = *status
	}
	if *pinned {
		q["pinned"] = "true"
	} else if *unpinned {
		q["pinned"] = "false"
	}
	raw, err := c.Get("/api/quests", q)
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
	if *questline != "" {
		lid, err := c.ResolveQuestlineID(*questline)
		if err != nil {
			return 1, err
		}
		filtered := items[:0]
		for _, it := range items {
			id, ok := AsInt64(it["questline_id"])
			if lid == nil {
				if it["questline_id"] == nil {
					filtered = append(filtered, it)
				}
			} else if ok && id == *lid {
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
		fmt.Println(FmtQuestLine(it))
	}
	return 0, nil
}

func cmdShow(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	raw, err := c.Get(fmt.Sprintf("/api/quests/%d", id), nil)
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, q, "")
	} else {
		fmt.Println(FmtQuestDetail(q))
	}
	return 0, nil
}

func cmdAdd(c *Client, args []string) (int, error) {
	fs := flagSet("add")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	pin := fs.Bool("pin", false, "")
	status := fs.String("status", "active", "")
	sig := fs.String("significance", "common", "")
	category := fs.String("category", "", "")
	questline := fs.String("questline", "", "")
	var steps multiFlag
	fs.Var(&steps, "step", "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return 1, fmt.Errorf("нужен TITLE")
	}
	title := strings.Join(pos, " ")
	body := map[string]any{
		"title": title, "description": *desc, "pinned": *pin,
		"status": *status, "significance": *sig,
	}
	if *category != "" {
		cid, err := c.ResolveCategoryID(*category)
		if err != nil {
			return 1, err
		}
		body["category_id"] = cid
	}
	if *questline != "" {
		lid, err := c.ResolveQuestlineID(*questline)
		if err != nil {
			return 1, err
		}
		body["questline_id"] = lid
	}
	if len(steps) > 0 {
		arr := make([]map[string]any, 0, len(steps))
		for _, s := range steps {
			arr = append(arr, map[string]any{"title": s, "progress_current": 0, "progress_total": 1})
		}
		body["steps"] = arr
	}
	raw, err := c.Post("/api/quests", nil, body)
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, q, "")
	} else {
		id, _ := AsInt64(q["id"])
		fmt.Printf("создан #%d: %s\n", id, AsString(q["title"]))
	}
	return 0, nil
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func cmdSet(c *Client, args []string) (int, error) {
	id, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("set")
	title := fs.String("title", "", "")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	category := fs.String("category", "", "")
	questline := fs.String("questline", "", "")
	sig := fs.String("significance", "", "")
	// track presence via Visit
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
	if set["questline"] {
		lid, err := c.ResolveQuestlineID(*questline)
		if err != nil {
			return 1, err
		}
		body["questline_id"] = lid
	}
	if set["significance"] {
		body["significance"] = *sig
	}
	if len(body) == 0 {
		return 1, fmt.Errorf("нечего менять: укажи --title/--description/--category/--questline/--significance")
	}
	raw, err := c.Patch(fmt.Sprintf("/api/quests/%d", id), nil, body)
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, q, "")
	} else {
		fmt.Println(FmtQuestDetail(q))
	}
	return 0, nil
}

func cmdPin(c *Client, args []string, forceOff bool) (int, error) {
	id, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	off := forceOff
	if !forceOff {
		fs := flagSet("pin")
		o := fs.Bool("off", false, "")
		if err := fs.Parse(rest); err != nil {
			return 2, nil
		}
		off = *o
	}
	raw, err := c.Patch(fmt.Sprintf("/api/quests/%d", id), nil, map[string]any{"pinned": !off})
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, q, "")
	} else {
		fmt.Printf("#%d pinned=%v\n", id, AsBool(q["pinned"]))
	}
	return 0, nil
}

func cmdStatus(c *Client, args []string, fixed string) (int, error) {
	id, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	status := fixed
	if status == "" {
		if len(rest) < 1 {
			return 1, fmt.Errorf("нужен STATUS")
		}
		status = rest[0]
	}
	raw, err := c.Patch(fmt.Sprintf("/api/quests/%d", id), nil, map[string]any{"status": status})
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, q, "")
	} else {
		fmt.Printf("#%d → %s\n", id, AsString(q["status"]))
	}
	return 0, nil
}

func cmdDelete(c *Client, args []string) (int, error) {
	id, _, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	_, err = c.Delete(fmt.Sprintf("/api/quests/%d", id), nil)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, map[string]any{"ok": true, "deleted": id}, "")
	} else {
		fmt.Printf("удалён #%d\n", id)
	}
	return 0, nil
}
