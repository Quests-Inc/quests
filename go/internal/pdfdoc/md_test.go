package pdfdoc

import "testing"

func TestHTMLDisclosure(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		summary string
		ok      bool
	}{
		{"plain opener", "<details>", "", true},
		{"opener with open attr", "<details open>", "", true},
		{"closer", "</details>", "", true},
		{
			"summary line from a real note",
			"<details>\n<summary>Таблица переходов — текстовая расшифровка графа</summary>",
			"Таблица переходов — текстовая расшифровка графа",
			true,
		},
		{"closer after summary", "<summary>Заголовок</summary>\n</details>", "Заголовок", true},
		{"two summaries join", "<summary>Один</summary>\n<summary>Два</summary>", "Один · Два", true},
		{"unknown tag stays literal", "<div class=x>текст</div>", "", false},
		{"stray text stays literal", "<details>\nкакой-то сырой текст\n</details>", "", false},
		{"unrelated html", "<span>hi</span>", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary, ok := htmlDisclosure(c.raw)
			if ok != c.ok || summary != c.summary {
				t.Fatalf("htmlDisclosure(%q) = (%q, %v), want (%q, %v)", c.raw, summary, ok, c.summary, c.ok)
			}
		})
	}
}

// A note with details/mermaid must render to a complete PDF: details expand with a bold
// summary line, the mermaid source stays as a code block with an explicit marker.
func TestRenderDetailsAndMermaid(t *testing.T) {
	out := render(t, Document{
		Title:  "Расширения",
		Blocks: []Block{{Type: BlockMarkdown, Markdown: mdExtensionsSample}},
	})
	if len(out) < 3000 {
		t.Fatalf("suspiciously small PDF: %d bytes", len(out))
	}
}

const mdExtensionsSample = "## Граф состояний\n\n" +
	"<details>\n<summary>Таблица переходов — текстовая расшифровка графа</summary>\n\n" +
	"| Состояние | Действие | Следующее |\n|---|---|---|\n| s0, sno=0 | Ожидание | s0 |\n| s0, sno=1 | Загрузить A и B | s1 |\n\n" +
	"</details>\n\n" +
	"```mermaid\nflowchart TD\n  A[Стенд] --> B{Живой?}\n  B -- да --> C[Продолжаем]\n  B -- нет --> D[Чиним]\n```\n\n" +
	"```MERMAID\nsequenceDiagram\n  Актёр->>Сервер: запрос\n```\n\n" +
	"- [x] готово\n- [ ] не готово\n"
