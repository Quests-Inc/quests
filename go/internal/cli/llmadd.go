package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// llmPreview mirrors the answer of POST /api/llm/actions/preview.
type llmPreview struct {
	NeedsClarification bool              `json:"needs_clarification"`
	ClarifyQuestion    string            `json:"clarify_question"`
	Batch              json.RawMessage   `json:"batch"`
	Preview            []llmPreviewEntry `json:"preview"`
}

type llmPreviewEntry struct {
	Index  int            `json:"index"`
	Action string         `json:"action"`
	IsNew  bool           `json:"is_new"`
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

// cmdLLMAdd turns free text into quest actions through the server's assistant
// (preview → confirm → apply), the same path the web form uses. The LLM
// provider, key and limits are the server's, not the CLI's.
func cmdLLMAdd(c *Client, args []string) (int, error) {
	fs := flagSet("llm-add")
	yes := fs.Bool("y", false, "")
	fs.BoolVar(yes, "yes", false, "")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		return 1, fmt.Errorf("нужен текст описания")
	}

	raw, err := c.Post("/api/llm/actions/preview", nil, map[string]any{"text": text})
	if err != nil {
		return 1, err
	}
	var pv llmPreview
	if err := json.Unmarshal(raw, &pv); err != nil {
		return 1, fmt.Errorf("разбор ответа ассистента: %w", err)
	}
	if pv.NeedsClarification {
		if c.AsJSON {
			Emit(true, map[string]any{"ok": false, "needs_clarification": true, "question": pv.ClarifyQuestion}, "")
		} else {
			fmt.Fprintln(os.Stderr, "уточнение:", pv.ClarifyQuestion)
		}
		return 2, nil
	}
	if len(pv.Preview) == 0 {
		return 1, fmt.Errorf("ассистент не предложил ни одного действия")
	}

	if !c.AsJSON {
		for _, e := range pv.Preview {
			fmt.Println(formatLLMPreviewEntry(e))
		}
	}
	if !*yes && !c.AsJSON && !confirm("применить? [y/N] ") {
		fmt.Println("отменено")
		return 1, nil
	}

	applied, err := c.Post("/api/llm/actions/apply", nil, map[string]any{"batch": pv.Batch})
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		var out any
		if err := json.Unmarshal(applied, &out); err != nil {
			return 1, err
		}
		Emit(true, out, "")
		return 0, nil
	}
	var res struct {
		Results []llmPreviewEntry `json:"results"`
	}
	if err := json.Unmarshal(applied, &res); err != nil {
		return 1, err
	}
	for _, e := range res.Results {
		fmt.Println("готово: " + formatLLMPreviewEntry(e))
	}
	return 0, nil
}

func formatLLMPreviewEntry(e llmPreviewEntry) string {
	state := e.After
	if state == nil {
		state = e.Before
	}
	verb := e.Action
	if e.IsNew {
		verb += " (новый)"
	}
	title := AsString(state["title"])
	if title == "" {
		title = AsString(state["name"])
	}
	return fmt.Sprintf("  %d. %s: %s", e.Index+1, verb, title)
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "y", "yes", "д", "да":
		return true
	}
	return false
}
