package pdfdoc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"testing"
)

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
const mermaidSource = "flowchart TD\n  A[Стенд] --> B{Живой?}\n  B -- да --> C[Продолжаем]"

// noisePNG is incompressible, so an embedded raster reliably grows the PDF bytes.
func noisePNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x*31 + y*7), G: uint8(y*17 + x*3), B: uint8(x * y), A: 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// The client raster is embedded where the trimmed source matches; a mismatched
// or broken raster silently keeps the source fallback.
func TestRenderMermaidFromClientPNG(t *testing.T) {
	md := "```mermaid\n" + mermaidSource + "\n```\n"
	fallback := render(t, Document{Title: "Д", Blocks: []Block{{Type: BlockMarkdown, Markdown: md}}})
	full := render(t, Document{
		Title:    "Д",
		Blocks:   []Block{{Type: BlockMarkdown, Markdown: md}},
		Diagrams: []Diagram{{Source: mermaidSource, PNG: noisePNG(256, 192)}},
	})
	if len(full) <= len(fallback) {
		t.Fatalf("embedded diagram must grow the PDF: fallback %d, full %d", len(fallback), len(full))
	}
	missing := render(t, Document{
		Title:    "Д",
		Blocks:   []Block{{Type: BlockMarkdown, Markdown: md}},
		Diagrams: []Diagram{{Source: "другая диаграмма", PNG: noisePNG(64, 48)}},
	})
	if len(missing) != len(fallback) {
		t.Fatalf("unmatched diagram must keep the fallback: %d vs %d", len(missing), len(fallback))
	}
	broken := render(t, Document{
		Title:    "Д",
		Blocks:   []Block{{Type: BlockMarkdown, Markdown: md}},
		Diagrams: []Diagram{{Source: mermaidSource, PNG: []byte("not a png")}},
	})
	if len(broken) != len(fallback) {
		t.Fatalf("broken raster must keep the fallback: %d vs %d", len(broken), len(fallback))
	}
}

// The index keys PNGs by the trimmed source and drops unusable entries.
func TestDiagramIndex(t *testing.T) {
	png := testPNG(32, 32)
	idx, err := Document{Diagrams: []Diagram{
		{Source: "  " + mermaidSource + "\n", PNG: png},
		{Source: "", PNG: png},
		{Source: "битая", PNG: nil},
		{Source: "огромная", PNG: make([]byte, MaxDiagramBytes+1)},
	}}.diagramIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 1 {
		t.Fatalf("want one usable entry, got %d", len(idx))
	}
	sum := sha256.Sum256([]byte(mermaidSource))
	if got, ok := idx[hex.EncodeToString(sum[:])]; !ok || len(got) != len(png) {
		t.Fatal("the trimmed source must key the raster")
	}
	small := Document{Diagrams: []Diagram{{Source: "x", PNG: []byte("p")}, {Source: "y", PNG: []byte("p")}}}
	if _, err := small.diagramIndex(); err != nil {
		t.Fatalf("small diagrams stay under the cap: %v", err)
	}
}

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
