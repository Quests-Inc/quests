package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
)

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
