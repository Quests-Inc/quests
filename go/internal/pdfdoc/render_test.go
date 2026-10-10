package pdfdoc

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

const sampleMD = "# Деплой на прод\n\n" +
	"Это **жирный**, *курсив*, ~~зачёркнутый~~ и `inline code` текст. Ссылка: [Quests](https://example.com/q). " +
	"Ссылка на заметку note=12 и квест quest=7, а ещё без метки note=99.\n" +
	"Мягкий перенос строки остаётся переносом.\n\n" +
	"## Список\n\n- первый пункт\n- второй пункт с длинным текстом, который обязательно должен перенестись на следующую строку, потому что он очень длинный\n  - вложенный пункт\n  - ещё один\n- [x] готово\n- [ ] не готово\n\n" +
	"1. шаг один\n2. шаг два\n\n" +
	"> Цитата про деплой: сначала проверить здоровье, потом выкатывать.\n> Вторая строка цитаты.\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"привет\") // комментарий\n}\n```\n\n" +
	"```\nпростой блок без языка с очень длинной строкой которая не помещается в ширину страницы и должна быть разрезана по колонкам аккуратно\n```\n\n" +
	"| Сервис | Порт | Заметка |\n|:--|--:|:-:|\n| api | 8765 | основной |\n| stt | 8766 | whisper, очень длинное описание в ячейке для проверки переноса |\n\n" +
	"---\n\n![схема|center](attachment=5)\n\n![нет](attachment=404)\n\n" +
	"Сверхдлинноеслово" + "ЭтоДлинноеСловоБезПробеловКотороеНеПомещаетсяВСтрокуИДолжноБытьРазрезано" + "ИЕщёРаз" + "ИЕщёРаз" + "ИЕщёРаз\n"

func testPNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 160, A: 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func loader(id int64) ([]byte, error) {
	if id == 5 {
		return testPNG(480, 240), nil
	}
	return nil, errors.New("not found")
}

func render(t *testing.T, doc Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(context.Background(), doc, loader, &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) || !bytes.Contains(buf.Bytes()[buf.Len()-64:], []byte("%%EOF")) {
		t.Fatalf("not a complete PDF (%d bytes)", buf.Len())
	}
	return buf.Bytes()
}

func TestRenderNoteLikeDocument(t *testing.T) {
	out := render(t, Document{
		Title:    "Деплой на прод",
		Footnote: "Выгрузка: 10 окт. 2026 г., 21:00",
		Blocks:   []Block{{Type: BlockMarkdown, Markdown: sampleMD}},
		Labels:   map[string]string{"note:12": "Мониторинг", "quest:7": "Починить CI"},
	})
	if len(out) < 5000 {
		t.Fatalf("suspiciously small PDF: %d bytes", len(out))
	}
	if p := os.Getenv("PDFDOC_OUT"); p != "" {
		_ = os.WriteFile(p, out, 0o644)
	}
}

func TestRenderQuestLikeDocument(t *testing.T) {
	out := render(t, Document{
		Title:    "Выкатить новую версию",
		Meta:     []MetaChip{{Text: "active", Color: "#268c46"}, {Text: "эпическое"}, {Text: "Работа"}, {Text: "1/3"}},
		Footnote: "Выгрузка: сегодня",
		Blocks: []Block{
			{Type: BlockMarkdown, Markdown: "Описание квеста с **важным** текстом и note=3."},
			{Type: BlockSteps, Label: "Шаги", Steps: []Step{
				{Title: "Собрать образы", Progress: "1/1", Done: true},
				{Title: "Накатить миграции и проверить, что всё хорошо работает после выкатки на прод", Progress: "0/2", Markdown: "- проверить `quests-server migrate`\n- посмотреть логи"},
				{Title: "Закрыть квест", Progress: "0/1"},
			}},
			{Type: BlockList, Label: "Вложения", Items: []string{"схема.png", "лог.txt"}},
			{Type: BlockFacts, Facts: []Fact{{"Создан (UTC+3)", "10 окт. 2026 г., 20:00"}, {"Срок (UTC+3)", "12 окт. 2026 г., 18:00"}}},
		},
		Labels: map[string]string{"note:3": "Релизы"},
	})
	if p := os.Getenv("PDFDOC_OUT"); p != "" {
		_ = os.WriteFile(strings.TrimSuffix(p, ".pdf")+"-quest.pdf", out, 0o644)
	}
}

func TestRenderEmptyAndHostile(t *testing.T) {
	render(t, Document{})
	render(t, Document{Title: "x", Blocks: []Block{{Type: BlockMarkdown, Markdown: "<script>alert(1)</script>\n\n<img src=\"file:///etc/passwd\">\n\n![x](http://127.0.0.1/a.png)\n\n![y](attachment=1)"}}})
	render(t, Document{Title: "x", Blocks: []Block{{Type: BlockMarkdown, Markdown: "\x00\x01 control \u202e rtl"}}})
}

func TestRenderTooLong(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 60000; i++ {
		sb.WriteString("строка номер многих строк подряд\n\n")
	}
	var buf bytes.Buffer
	err := Render(context.Background(), Document{Title: "x", Blocks: []Block{{Type: BlockMarkdown, Markdown: sb.String()}}}, nil, &buf)
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("want ErrTooLong, got %v", err)
	}
}

func TestRenderRespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := Render(ctx, Document{Title: "x", Blocks: []Block{{Type: BlockMarkdown, Markdown: "text"}}}, nil, &buf); err == nil {
		t.Fatal("a cancelled context must stop the render")
	}
}

func TestPrepareImageRejectsGarbageAndBombs(t *testing.T) {
	if _, _, _, _, err := prepareImage([]byte("not an image")); err == nil {
		t.Fatal("garbage must be rejected")
	}
	if _, _, _, _, err := prepareImage(make([]byte, MaxImageBytes+1)); err == nil {
		t.Fatal("oversized input must be rejected")
	}
}
