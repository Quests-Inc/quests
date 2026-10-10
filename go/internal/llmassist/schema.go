// Package llmassist is the Go port of the action-batch assistant: free text
// -> Groq structured JSON batch of quest actions -> dry-run preview -> apply.
// Same non-agentic philosophy as before (constrained decode, no tool-calling
// loop) — this package used to be a standalone Python aiohttp service
// (quests-llm); it's now in-process with the main API since Go has no heavy
// per-language dependency cost to isolate.
package llmassist

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

var actionKinds = []string{
	"create_questline",
	"create_quest",
	"add_step",
	"update_step",
	"delete_step",
	"update_quest",
}

var statusValues = []string{"active", "expired", "frozen", "completed", "failed", "archived"}

// CategoryHint is a live journal category for the system prompt appendix.
type CategoryHint struct {
	ID    int64
	Slug  string
	Label string
}

// PromptContext is per-request dynamic data injected into the system prompt.
type PromptContext struct {
	NowLocal   string
	TimeZone   string
	Categories []CategoryHint
	// NoClarify forbids needs_clarification: the user has already answered a
	// clarifying question, so the model must commit to its best reading.
	NoClarify bool
}

// DefaultPromptContext builds NowLocal/TZ from QUESTS_TZ / TZ / time.Local.
func DefaultPromptContext(categories []CategoryHint) PromptContext {
	tzName := strings.TrimSpace(os.Getenv("QUESTS_TZ"))
	if tzName == "" {
		tzName = strings.TrimSpace(os.Getenv("TZ"))
	}
	loc := time.Local
	if tzName != "" {
		if l, err := time.LoadLocation(tzName); err == nil {
			loc = l
		}
	} else {
		tzName = loc.String()
		if tzName == "Local" {
			tzName = "Europe/Moscow"
			if l, err := time.LoadLocation(tzName); err == nil {
				loc = l
			}
		}
	}
	now := time.Now().In(loc)
	return PromptContext{
		NowLocal:   now.Format("2006-01-02 15:04 Monday"),
		TimeZone:   tzName,
		Categories: categories,
	}
}

// ActionStep is an inline step for create_quest's steps list.
type ActionStep struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ProgressTotal   int    `json:"progress_total"`
	ProgressCurrent int    `json:"progress_current"`
}

// Action mirrors the flat action shape from the (former) Python schema —
// one struct covers all six action kinds, unused fields stay null.
type Action struct {
	Index  int    `json:"index"`
	Action string `json:"action"`

	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Status          *string `json:"status"`
	Significance    *string `json:"significance"`
	Pinned          *bool   `json:"pinned"`
	SortOrder       *int    `json:"sort_order"`
	DeadlineAt      *string `json:"deadline_at"`
	DurationSeconds *int    `json:"duration_seconds"`

	Category       *string `json:"category"`
	CategoryID     *int64  `json:"category_id"`
	Questline      *string `json:"questline"`
	QuestlineID    *int64  `json:"questline_id"`
	ClearQuestline bool    `json:"clear_questline"`
	Color          *string `json:"color"`
	Icon           *string `json:"icon"`

	QuestID         *int64       `json:"quest_id"`
	StepID          *int64       `json:"step_id"`
	ProgressCurrent *int         `json:"progress_current"`
	ProgressTotal   *int         `json:"progress_total"`
	Steps           []ActionStep `json:"steps"`

	QuestlineIDRef *int `json:"questline_id_ref"`
	QuestIDRef     *int `json:"quest_id_ref"`
}

type ActionBatch struct {
	NeedsClarification bool     `json:"needs_clarification"`
	ClarifyQuestion    string   `json:"clarify_question"`
	Actions            []Action `json:"actions"`
}

func actionJSONSchema() map[string]any {
	strOrNull := map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}}
	intOrNull := map[string]any{"anyOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "null"}}}
	boolOrNull := map[string]any{"anyOf": []any{map[string]any{"type": "boolean"}, map[string]any{"type": "null"}}}
	strMax := func(max int) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"type": "string", "maxLength": max}, map[string]any{"type": "null"}}}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"index":            map[string]any{"type": "integer", "minimum": 0},
			"action":           map[string]any{"type": "string", "enum": actionKinds},
			"title":            strMax(200),
			"description":      strOrNull,
			"status":           map[string]any{"anyOf": []any{map[string]any{"type": "string", "enum": statusValues}, map[string]any{"type": "null"}}},
			"significance":     strOrNull,
			"pinned":           boolOrNull,
			"sort_order":       intOrNull,
			"deadline_at":      strOrNull,
			"duration_seconds": intOrNull,
			"category":         strOrNull,
			"category_id":      intOrNull,
			"questline":        strOrNull,
			"questline_id":     intOrNull,
			"clear_questline":  map[string]any{"type": "boolean"},
			"color":            strOrNull,
			"icon":             strOrNull,
			"quest_id":         intOrNull,
			"step_id":          intOrNull,
			"progress_current": intOrNull,
			"progress_total":   intOrNull,
			"steps": map[string]any{
				"anyOf": []any{
					map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"title":            map[string]any{"type": "string", "minLength": 1, "maxLength": 200},
								"description":      map[string]any{"type": "string"},
								"progress_total":   map[string]any{"type": "integer", "minimum": 1},
								"progress_current": map[string]any{"type": "integer", "minimum": 0},
							},
							"required": []any{"title", "description", "progress_total", "progress_current"},
						},
						"maxItems": 20,
					},
					map[string]any{"type": "null"},
				},
			},
			"questline_id_ref": intOrNull,
			"quest_id_ref":     intOrNull,
		},
		"required": []any{
			"index", "action", "title", "description", "status", "significance",
			"pinned", "sort_order", "deadline_at", "duration_seconds",
			"category", "category_id", "questline", "questline_id",
			"clear_questline", "color", "icon", "quest_id", "step_id",
			"progress_current", "progress_total", "steps",
			"questline_id_ref", "quest_id_ref",
		},
	}
}

// ActionBatchJSONSchema is the response_format.json_schema.schema sent to Groq.
func ActionBatchJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"needs_clarification": map[string]any{"type": "boolean"},
			"clarify_question":    map[string]any{"type": "string"},
			"actions": map[string]any{
				"type":     "array",
				"items":    actionJSONSchema(),
				"maxItems": 20,
			},
		},
		"required": []any{"needs_clarification", "clarify_question", "actions"},
	}
}

// SystemPrompt builds the action-batch system message, including live clock
// and journal categories so create_quest doesn't land in «Без раздела».
func SystemPrompt(pc PromptContext) string {
	schema, _ := json.Marshal(ActionBatchJSONSchema())
	var b strings.Builder
	b.WriteString("Ты преобразуешь запрос пользователя в список действий (actions) над ")
	b.WriteString("журналом квестов Quests.\n")
	b.WriteString("СИНТАКСИС ССЫЛОК: quest=N / questline=N / step=N — это точные числовые id, ")
	b.WriteString("подставленные клиентом через автокомплит/копирование по УЖЕ СУЩЕСТВУЮЩЕЙ сущности. ")
	b.WriteString("Если видишь такой тег — просто скопируй N в соответствующее *_id поле, ")
	b.WriteString("ничего не резолвь и не выдумывай.\n")
	b.WriteString("Любое название БЕЗ такого тега (обычный текст) — это title НОВОЙ сущности, ")
	b.WriteString("которую нужно создать (create_quest/create_questline/новый шаг). ")
	b.WriteString("Если голый текст совпадает с title, который создаётся ДРУГИМ action'ом в ")
	b.WriteString("ЭТОМ ЖЕ списке — это ссылка на него: используй questline_id_ref/quest_id_ref ")
	b.WriteString("с индексом создающего action'а, а НЕ questline_id/quest_id и НЕ ")
	b.WriteString("questline/category (те поля — только для имени НОВОГО объекта при его ")
	b.WriteString("создании или для резолва имени уже существующего объекта по подстроке).\n")
	b.WriteString("Действия и когда их использовать:\n")
	b.WriteString("  • create_questline — создать новый квестлайн. title обязателен. ")
	b.WriteString("Категорию квестлайна задай через category (slug или label) или category_id.\n")
	b.WriteString("  • create_quest — создать НОВЫЙ квест с нуля. steps — только если ")
	b.WriteString("пользователь явно перечислил шаги (запятая, «и», нумерация); иначе steps=null ")
	b.WriteString("(не выдумывай подготовку/проверку/коммит). Если квест создаётся ")
	b.WriteString("СРАЗУ ВНУТРИ квестлайна, который тоже создаётся в этом же запросе — ")
	b.WriteString("ОБЯЗАТЕЛЬНО проставь questline_id_ref на индекс create_questline ")
	b.WriteString("action'а. Если квестлайн НЕ задан — ОБЯЗАТЕЛЬНО проставь category ")
	b.WriteString("(slug/label из списка ниже) или category_id; null только если раздел ")
	b.WriteString("действительно неясен.\n")
	b.WriteString("  • update_quest — изменить СУЩЕСТВУЮЩИЙ квест (обязателен quest_id из ")
	b.WriteString("тега quest=N, или quest_id_ref если квест создаётся в этом же батче): ")
	b.WriteString("статус, title, pinned, значимость, category, и ГЛАВНОЕ — прикрепление к квестлайну ")
	b.WriteString("(questline_id из тега questline=N, questline_id_ref если квестлайн ")
	b.WriteString("создаётся тут же, или questline как имя уже существующего для резолва ")
	b.WriteString("по подстроке; clear_questline=true — отвязать).\n")
	b.WriteString("  • add_step — добавить НОВЫЙ шаг внутрь существующего квеста (quest_id ")
	b.WriteString("из тега quest=N). title обязателен, steps не используется.\n")
	b.WriteString("  • update_step / delete_step — quest_id + step_id (оба из тегов).\n")
	b.WriteString("Примеры:\n")
	b.WriteString("  \"создай квестлайн Бэкапы и закинь туда quest=42\" →\n")
	b.WriteString("    [{index:0, action:create_questline, title:\"Бэкапы\"}, ")
	b.WriteString("{index:1, action:update_quest, quest_id:42, questline_id_ref:0}]\n")
	b.WriteString("  \"перенеси quest=5 в questline=3, поставь completed\" →\n")
	b.WriteString("    [{index:0, action:update_quest, quest_id:5, questline_id:3, ")
	b.WriteString("status:\"completed\"}]\n")
	b.WriteString("  \"quest=9 сделай completed и создай квестлайн Инфраструктура, закинь ")
	b.WriteString("его туда\" →\n")
	b.WriteString("    [{index:0, action:update_quest, quest_id:9, status:\"completed\"}, ")
	b.WriteString("{index:1, action:create_questline, title:\"Инфраструктура\"}, ")
	b.WriteString("{index:2, action:update_quest, quest_id:9, questline_id_ref:1}]\n")
	b.WriteString("  \"создай квестлайн Мониторинг с квестом Настроить графану\" →\n")
	b.WriteString("    [{index:0, action:create_questline, title:\"Мониторинг\"}, ")
	b.WriteString("{index:1, action:create_quest, title:\"Настроить графану\", ")
	b.WriteString("questline_id_ref:0}]\n")
	b.WriteString("Верни ОДИН JSON-объект по схеме, без пояснений вне JSON. Поля, не нужные ")
	b.WriteString("для конкретного action, оставляй null (clear_questline=false по ")
	b.WriteString("умолчанию, steps=null кроме create_quest с явными шагами).\n")
	b.WriteString("status один из: active, expired, frozen, completed, failed, archived.\n")
	b.WriteString("deadline_at — ISO-8601 в локальном времени пользователя или с offset; ")
	b.WriteString("считай относительно «сейчас» ниже (не UTC, если пользователь не сказал GMT).\n")
	b.WriteString("needs_clarification=true (actions=[]) только если запрос реально ")
	b.WriteString("неоднозначен (непонятно, какой квест/шаг/квестлайн имеется в виду и ")
	b.WriteString("тег/имя не помогает установить это однозначно).\n")
	if pc.NoClarify {
		b.WriteString("УТОЧНЯТЬ БОЛЬШЕ НЕЛЬЗЯ: пользователь уже ответил на уточнение. ")
		b.WriteString("needs_clarification ВСЕГДА false; если что-то неясно — выбери самое ")
		b.WriteString("вероятное толкование и составь план (пользователь увидит его и сможет ")
		b.WriteString("отклонить).\n")
	}

	b.WriteString("\nКОНТЕКСТ СЕЙЧАС:\n")
	fmt.Fprintf(&b, "  Локальное время: %s\n", pc.NowLocal)
	fmt.Fprintf(&b, "  Часовой пояс: %s (все относительные сроки — от этого якоря).\n", pc.TimeZone)
	b.WriteString("РАЗДЕЛЫ (category / category_id) — выбирай из списка; для create_quest ")
	b.WriteString("без квестлайна почти всегда нужен раздел:\n")
	if len(pc.Categories) == 0 {
		b.WriteString("  (список пуст — category=null)\n")
	} else {
		for _, c := range pc.Categories {
			fmt.Fprintf(&b, "  • id=%d slug=%q label=%q\n", c.ID, c.Slug, c.Label)
		}
	}
	b.WriteString("\nJSON Schema:\n")
	b.Write(schema)
	return b.String()
}
