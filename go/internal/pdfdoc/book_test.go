package pdfdoc

import (
	"bytes"
	"context"
	"github.com/go-pdf/fpdf"
	"github.com/yuin/goldmark/text"
	"os"
	"strings"
	"testing"
)

func TestRenderBook(t *testing.T) {
	doc := Document{Title: "Справочник Quests", Book: true, Footnote: "Тестовая книга", Labels: map[string]string{"note:2": "Подключение"}, Chapters: []Chapter{
		{NoteID: 1, Title: "Введение", Depth: 0, Markdown: "Вводная часть. Перейти к note=2#setup.\n\nК таблице note=2#checks/ok."},
		{NoteID: 2, Title: "Подключение", Depth: 1, Markdown: "<!-- span-id: setup -->\n## Питание\n\nПодключите **контроллер**.\n<!-- /span-id: setup -->\n\n![Схема|center](attachment=5)\n\n<!-- table-id: checks -->\n| Проверка | Результат |\n|---|---|\n| <!-- row-id: ok -->Питание | Исправно |\n\n$$\\frac{3}{8}$$"},
		{NoteID: 3, Title: "Диагностика", Depth: 2, Markdown: strings.Repeat("Проверить подключение и состояние службы.\n\n", 70)},
		{NoteID: 4, Title: "Восстановление", Depth: 1, Markdown: "Сохраните копию перед восстановлением."},
	}}
	out := render(t, doc)
	if !bytes.Contains(out, []byte("/Outlines")) || !bytes.Contains(out, []byte("/Subtype /Link")) {
		t.Fatal("missing outlines or navigation")
	}
	if p := os.Getenv("PDFBOOK_OUT"); p != "" {
		if err := os.WriteFile(p, out, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestBookRejectsInvalidHierarchyAndBudget(t *testing.T) {
	for _, doc := range []Document{
		{Book: true, Chapters: []Chapter{{NoteID: 1, Depth: 1}}},
		{Book: true, Chapters: []Chapter{{NoteID: 1}, {NoteID: 1, Depth: 1}}},
		{Book: true, Chapters: []Chapter{{NoteID: 1}, {NoteID: 2, Depth: 3}}},
		{Book: true, Chapters: []Chapter{{NoteID: 1, Markdown: strings.Repeat("x", MaxMarkdownBytes+1)}}},
	} {
		var b bytes.Buffer
		if err := Render(context.Background(), doc, nil, &b); err == nil {
			t.Fatal("invalid book accepted")
		}
	}
}

func TestBookWithMultiPageContents(t *testing.T) {
	chapters := []Chapter{{NoteID: 1, Title: "Root"}, {NoteID: 2, Title: "Chapter", Depth: 1}}
	for i := 3; i < 90; i++ {
		chapters = append(chapters, Chapter{NoteID: int64(i), Title: strings.Repeat("Длинный заголовок раздела ", 4), Depth: 2, Markdown: "Текст."})
	}
	render(t, Document{Title: "Книга с большим оглавлением", Book: true, Chapters: chapters})
}

func TestPDFLinkColors(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	if err := registerFonts(pdf); err != nil {
		t.Fatal(err)
	}
	r := &Renderer{ctx: context.Background(), pdf: pdf, labels: map[string]string{"note:1": "Inside", "note:2": "Outside"}, bookLinks: map[string]int{"note:1": 1}, referenceBase: "https://quests.example/"}
	src := []byte("note=1 note=2 [Website](https://example.com) <https://example.org>")
	doc := md.Parser().Parse(text.NewReader(src))
	toks := r.inlineTokens(doc.FirstChild(), src, bodyStyle())
	found := map[string]bool{}
	for _, tok := range toks {
		switch tok.text {
		case "Inside":
			if tok.st.color != colLink || tok.st.internal != 1 || tok.st.link != "" {
				t.Fatal("internal reference must remain blue and internal")
			}
			found["internal"] = true
		case "Outside":
			if tok.st.color != colExternalLink || tok.st.link != "https://quests.example/?note=2" {
				t.Fatal("outside reference must be red and link to site")
			}
			found["outside"] = true
		case "Website", "https://example.org":
			if tok.st.color != colExternalLink || tok.st.link == "" {
				t.Fatal("external URL must be red")
			}
			found[tok.text] = true
		}
	}
	if len(found) != 4 {
		t.Fatalf("missing test links: %v", found)
	}
}

func TestGothicKeepsCyrillicAndFallsBackForSymbols(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	if err := registerFonts(pdf); err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Font = "gothic"
	r := Renderer{ctx: context.Background(), pdf: pdf, options: o}
	var tokens []token
	r.textTokens(r.bodyStyle(), "Кириллица →", &tokens)
	gothic, fallback := false, false
	for _, tok := range tokens {
		if tok.text == "Кириллица" && tok.st.fam == famGothic {
			gothic = true
		}
		if tok.text == "→" && tok.st.fam == famSerif {
			fallback = true
		}
	}
	if !gothic || !fallback {
		t.Fatal("Cyrillic or symbol fallback lost")
	}
}
