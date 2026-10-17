package cli

import (
	"fmt"
	"strings"
)

func FmtQuestLine(q map[string]any) string {
	pin := " "
	if AsBool(q["pinned"]) {
		pin = "★"
	}
	id, _ := AsInt64(q["id"])
	title := AsString(q["title"])
	if title == "" {
		title = "?"
	}
	status := AsString(q["status"])
	progress := AsString(q["progress_label"])
	var extras []string
	if cat := AsString(q["category_slug"]); cat == "" {
		cat = AsString(q["category_label"])
		if cat != "" {
			extras = append(extras, cat)
		}
	} else {
		extras = append(extras, cat)
	}
	if line := AsString(q["questline_title"]); line != "" {
		extras = append(extras, "⟶"+line)
	}
	suffix := ""
	if len(extras) > 0 {
		suffix = "  [" + strings.Join(extras, ", ") + "]"
	}
	return fmt.Sprintf("%s %4d  %-10s  %-8s  %s%s", pin, id, status, progress, title, suffix)
}

func FmtQuestDetail(q map[string]any) string {
	id, _ := AsInt64(q["id"])
	lines := []string{
		fmt.Sprintf("#%d  %s", id, AsString(q["title"])),
		fmt.Sprintf("  status:       %s", AsString(q["status"])),
		fmt.Sprintf("  significance: %s", AsString(q["significance"])),
		fmt.Sprintf("  pinned:       %v", AsBool(q["pinned"])),
		fmt.Sprintf("  progress:     %s", AsString(q["progress_label"])),
	}
	var catBits []string
	if _, ok := AsInt64(q["category_id"]); ok && q["category_id"] != nil {
		catBits = append(catBits, fmt.Sprintf("id=%v", q["category_id"]))
	}
	if s := AsString(q["category_slug"]); s != "" {
		catBits = append(catBits, s)
	}
	if s := AsString(q["category_label"]); s != "" {
		catBits = append(catBits, s)
	}
	cat := "—"
	if len(catBits) > 0 {
		cat = strings.Join(catBits, " ")
	}
	lines = append(lines, "  category:     "+cat)
	if qid, ok := AsInt64(q["questline_id"]); ok && q["questline_id"] != nil {
		lines = append(lines, fmt.Sprintf("  questline:    #%d %s", qid, strings.TrimSpace(AsString(q["questline_title"]))))
	} else {
		lines = append(lines, "  questline:    —")
	}
	if d := AsString(q["deadline_at"]); d != "" && d != "<nil>" {
		lines = append(lines, "  deadline:     "+d)
	}
	if d := AsString(q["description"]); d != "" {
		lines = append(lines, "  description:  "+d)
	}
	steps, _ := q["steps"].([]any)
	if len(steps) > 0 {
		lines = append(lines, "  steps:")
		for _, s := range steps {
			sm, _ := s.(map[string]any)
			sid, _ := AsInt64(sm["id"])
			mark := " "
			if AsBool(sm["done"]) {
				mark = "✓"
			}
			lines = append(lines, fmt.Sprintf("    %s [%d] %s  %v/%v",
				mark, sid, AsString(sm["title"]), sm["progress_current"], sm["progress_total"]))
		}
	}
	return strings.Join(lines, "\n")
}

func FmtCategoryLine(c map[string]any) string {
	id, _ := AsInt64(c["id"])
	return fmt.Sprintf("%4d  %-12s  %s  %s", id, AsString(c["slug"]), AsString(c["color"]), AsString(c["label"]))
}

func FmtQuestlineLine(l map[string]any) string {
	id, _ := AsInt64(l["id"])
	cat := AsString(l["category_slug"])
	if cat == "" {
		cat = "—"
	}
	return fmt.Sprintf("%4d  %-10s  %s  %s", id, cat, AsString(l["color"]), AsString(l["title"]))
}

func FmtQuestlineDetail(l map[string]any) string {
	id, _ := AsInt64(l["id"])
	lines := []string{
		fmt.Sprintf("#%d  %s", id, AsString(l["title"])),
		fmt.Sprintf("  color:   %s", AsString(l["color"])),
		fmt.Sprintf("  icon:    %s", AsString(l["icon"])),
	}
	if cid, ok := AsInt64(l["category_id"]); ok && l["category_id"] != nil {
		lines = append(lines, fmt.Sprintf("  category: #%d %s %s", cid, AsString(l["category_slug"]), AsString(l["category_label"])))
	} else {
		lines = append(lines, "  category: —")
	}
	if d := AsString(l["description"]); d != "" {
		lines = append(lines, "  description: "+d)
	}
	return strings.Join(lines, "\n")
}

func FmtNoteLine(n map[string]any) string {
	pin := " "
	if AsBool(n["pinned"]) {
		pin = "★"
	}
	id, _ := AsInt64(n["id"])
	parent := "—"
	if pid, ok := AsInt64(n["parent_id"]); ok && n["parent_id"] != nil {
		parent = fmt.Sprintf("#%d", pid)
	}
	return fmt.Sprintf("%s %4d  parent=%-6s  %s", pin, id, parent, AsString(n["title"]))
}

func FmtNoteDetail(n map[string]any) string {
	id, _ := AsInt64(n["id"])
	lines := []string{
		fmt.Sprintf("#%d  %s", id, AsString(n["title"])),
		fmt.Sprintf("  pinned:  %v", AsBool(n["pinned"])),
	}
	if pid, ok := AsInt64(n["parent_id"]); ok && n["parent_id"] != nil {
		lines = append(lines, fmt.Sprintf("  parent:  #%d", pid))
	} else {
		lines = append(lines, "  parent:  —")
	}
	if d := AsString(n["description"]); d != "" {
		lines = append(lines, "  body:\n"+d)
	}
	return strings.Join(lines, "\n")
}
