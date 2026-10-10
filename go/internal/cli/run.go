package cli

import (
	"encoding/json"
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

func cmdStep(c *Client, args []string) (int, error) {
	id, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("step")
	stepID := fs.Int64("step-id", 0, "")
	title := fs.String("title", "", "")
	inc := fs.Int("inc", -1, "")
	setN := fs.Int("set", -1, "")
	done := fs.Bool("done", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2, nil
	}
	raw, err := c.Get(fmt.Sprintf("/api/quests/%d", id), nil)
	if err != nil {
		return 1, err
	}
	q, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	stepsAny, _ := q["steps"].([]any)
	if len(stepsAny) == 0 {
		return 1, fmt.Errorf("у квеста нет шагов")
	}
	var step map[string]any
	if *stepID > 0 {
		for _, s := range stepsAny {
			sm := s.(map[string]any)
			sid, _ := AsInt64(sm["id"])
			if sid == *stepID {
				step = sm
				break
			}
		}
		if step == nil {
			return 1, fmt.Errorf("шаг %d не найден", *stepID)
		}
	} else if *title != "" {
		needle := strings.ToLower(*title)
		var matches []map[string]any
		for _, s := range stepsAny {
			sm := s.(map[string]any)
			if strings.Contains(strings.ToLower(AsString(sm["title"])), needle) {
				matches = append(matches, sm)
			}
		}
		if len(matches) == 0 {
			return 1, fmt.Errorf("шаг с title≈%q не найден", *title)
		}
		if len(matches) > 1 {
			return 1, fmt.Errorf("несколько шагов; укажи --step-id")
		}
		step = matches[0]
	} else {
		for _, s := range stepsAny {
			sm := s.(map[string]any)
			if !AsBool(sm["done"]) {
				step = sm
				break
			}
		}
		if step == nil {
			step = stepsAny[len(stepsAny)-1].(map[string]any)
		}
	}
	cur, _ := AsInt64(step["progress_current"])
	total, _ := AsInt64(step["progress_total"])
	if total < 1 {
		total = 1
	}
	var newCur int64
	switch {
	case *done:
		newCur = total
	case *setN >= 0:
		newCur = int64(*setN)
	case *inc >= 0:
		newCur = cur + int64(*inc)
	default:
		newCur = cur + 1
	}
	sid, _ := AsInt64(step["id"])
	raw, err = c.Patch(fmt.Sprintf("/api/quests/%d/steps/%d", id, sid), nil, map[string]any{"progress_current": newCur})
	if err != nil {
		return 1, err
	}
	updated, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, updated, "")
		return 0, nil
	}
	for _, s := range updated["steps"].([]any) {
		sm := s.(map[string]any)
		xid, _ := AsInt64(sm["id"])
		if xid == sid {
			fmt.Printf("#%d step [%d] %s: %v/%v  (%s)\n",
				id, sid, AsString(sm["title"]), sm["progress_current"], sm["progress_total"], AsString(updated["progress_label"]))
			break
		}
	}
	return 0, nil
}

func cmdStepAdd(c *Client, args []string) (int, error) {
	id, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("step-add")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	total := fs.Int("total", 1, "")
	progress := fs.Int("progress", 0, "")
	sortOrder := fs.Int("sort-order", -1, "")
	quiet := fs.Bool("quiet", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2, nil
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return 1, fmt.Errorf("нужен TITLE")
	}
	body := map[string]any{
		"title": strings.Join(pos, " "), "progress_current": *progress, "progress_total": max(1, *total),
	}
	if *desc != "" {
		body["description"] = *desc
	}
	if *sortOrder >= 0 {
		body["sort_order"] = *sortOrder
	}
	q := map[string]string{}
	if *quiet {
		q["quiet"] = "1"
	}
	raw, err := c.Post(fmt.Sprintf("/api/quests/%d/steps", id), q, body)
	if err != nil {
		return 1, err
	}
	updated, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, updated, "")
		return 0, nil
	}
	var maxID int64
	var added map[string]any
	for _, s := range updated["steps"].([]any) {
		sm := s.(map[string]any)
		sid, _ := AsInt64(sm["id"])
		if sid >= maxID {
			maxID = sid
			added = sm
		}
	}
	fmt.Printf("#%d +step [%d] %s  (%s)\n", id, maxID, AsString(added["title"]), AsString(updated["progress_label"]))
	return 0, nil
}

func cmdStepEdit(c *Client, args []string) (int, error) {
	qid, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	sid, rest, err := parseID(rest, "step_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("step-edit")
	title := fs.String("title", "", "")
	desc := fs.String("d", "", "")
	fs.StringVar(desc, "description", "", "")
	total := fs.Int("total", -1, "")
	setN := fs.Int("set", -1, "")
	sortOrder := fs.Int("sort-order", -999999, "")
	quiet := fs.Bool("quiet", false, "")
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
	if *total >= 0 {
		body["progress_total"] = max(1, *total)
	}
	if *setN >= 0 {
		body["progress_current"] = *setN
	}
	if *sortOrder != -999999 {
		body["sort_order"] = *sortOrder
	}
	if len(body) == 0 {
		return 1, fmt.Errorf("укажи хотя бы одно поле")
	}
	q := map[string]string{}
	if *quiet {
		q["quiet"] = "1"
	}
	raw, err := c.Patch(fmt.Sprintf("/api/quests/%d/steps/%d", qid, sid), q, body)
	if err != nil {
		return 1, err
	}
	updated, err := DecodeMap(raw)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, updated, "")
		return 0, nil
	}
	for _, s := range updated["steps"].([]any) {
		sm := s.(map[string]any)
		xid, _ := AsInt64(sm["id"])
		if xid == sid {
			fmt.Printf("#%d step [%d] %s: %v/%v  (%s)\n",
				qid, sid, AsString(sm["title"]), sm["progress_current"], sm["progress_total"], AsString(updated["progress_label"]))
			break
		}
	}
	return 0, nil
}

func cmdStepRm(c *Client, args []string) (int, error) {
	qid, rest, err := parseID(args, "quest_id")
	if err != nil {
		return 1, err
	}
	sid, rest, err := parseID(rest, "step_id")
	if err != nil {
		return 1, err
	}
	fs := flagSet("step-rm")
	quiet := fs.Bool("quiet", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2, nil
	}
	q := map[string]string{}
	if *quiet {
		q["quiet"] = "1"
	}
	raw, err := c.Delete(fmt.Sprintf("/api/quests/%d/steps/%d", qid, sid), q)
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		if raw == nil {
			Emit(true, map[string]any{"ok": true}, "")
		} else {
			m, _ := DecodeMap(raw)
			Emit(true, m, "")
		}
		return 0, nil
	}
	label := ""
	if raw != nil {
		if m, err := DecodeMap(raw); err == nil {
			label = AsString(m["progress_label"])
		}
	}
	fmt.Printf("#%d −step %d  (%s)\n", qid, sid, label)
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

// cmdProgress prints {"progress": <closed steps>, "total": <steps>} for a
// quest — the poll-mode output a check_command of a *parent* step can return to
// mirror a child quest (see docs/cli.md). It deliberately omits "description"
// so the parent step keeps its own text (usually the quest=N link).
func cmdProgress(c *Client, args []string) (int, error) {
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
	steps, _ := q["steps"].([]any)
	done := 0
	for _, s := range steps {
		if m, ok := s.(map[string]any); ok {
			if d, _ := m["done"].(bool); d {
				done++
			}
		}
	}
	total := len(steps)
	if total < 1 {
		total = 1
	}
	out, err := json.Marshal(map[string]int{"progress": done, "total": total})
	if err != nil {
		return 1, err
	}
	fmt.Println(string(out))
	return 0, nil
}
