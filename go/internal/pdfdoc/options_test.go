package pdfdoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
)

func TestOptionsValidation(t *testing.T) {
	var decoded Document
	if err := json.Unmarshal([]byte(`{"options":{"left":0}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Options.Left != 0 || decoded.Options.FontSize != 10 || decoded.Options.Width != 210 {
		t.Fatal("partial JSON must preserve explicit zero and default omitted fields")
	}
	for _, modify := range []func(*Options){
		func(o *Options) { o.Width = 20 }, func(o *Options) { o.FontSize = math.NaN() }, func(o *Options) { o.Top = 100; o.Bottom = 100; o.Height = 210 },
		func(o *Options) { o.Left = -1 }, func(o *Options) { o.LineSpacing = 0 }, func(o *Options) { o.Font = "unknown" }, func(o *Options) { o.TextColor = "red" },
		func(o *Options) { o.NumberFormat = "roman" }, func(o *Options) { o.NumberPosition = "top"; o.Top = 3 }, func(o *Options) { o.BackgroundOpacity = 2 },
		func(o *Options) { o.BackgroundImage = -1 }, func(o *Options) { o.Binding = 100; o.Left = 60; o.Right = 60 },
	} {
		o := DefaultOptions()
		modify(&o)
		var invalid *InvalidOptionsError
		if !errors.As(o.validate(), &invalid) {
			t.Fatalf("accepted invalid options: %+v", o)
		}
	}
}
func TestCustomGeometryAndMirroredMargins(t *testing.T) {
	o := DefaultOptions()
	o.Width = 148
	o.Height = 210
	o.Landscape = true
	o.Left = 24
	o.Right = 12
	o.Binding = 6
	o.MirrorMargins = true
	o.Font = "sans"
	o.FontSize = 12
	o.LineSpacing = 1.8
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	r := Renderer{options: o}
	if r.pageWidth() != 210 || r.pageHeight() != 148 || r.contentWidth() != 168 || r.leftMargin(1) != 30 || r.leftMargin(2) != 12 {
		t.Fatal("wrong orientation or binding geometry")
	}
	if r.bodyStyle().fam != famSans || r.bodyStyle().size != 12 || math.Abs(r.lineHeight(12)-12*ptMM*1.8) > .001 {
		t.Fatal("typography not applied")
	}
}
func TestRenderCustomBackground(t *testing.T) {
	o := DefaultOptions()
	o.Width = 148
	o.Height = 210
	o.BackgroundImage = 5
	o.BackgroundOpacity = .5
	o.BackgroundFit = "cover"
	o.FontSize = 12
	o.NumberFormat = "page"
	o.NumberPosition = "top"
	o.NumberAlign = "right"
	o.BackgroundColor = "#e8dfc5"
	out := render(t, Document{Title: "Проверка оформления", Options: &o, Blocks: []Block{{Type: BlockMarkdown, Markdown: strings.Repeat("Текст на фоне с заданными полями.\n\n", 60)}}})
	if !bytes.Contains(out, []byte("/ExtGState")) || !bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Fatal("background opacity or image missing")
	}
	var b bytes.Buffer
	if err := Render(context.Background(), Document{Options: &o}, nil, &b); err == nil {
		t.Fatal("unavailable requested background silently ignored")
	}
}

// Optional visual QA fixture using a user-supplied attachment, never copied into the repository.
func TestParchmentVisualFixture(t *testing.T) {
	path := os.Getenv("PDFOPTIONS_BACKGROUND")
	if path == "" {
		t.Skip("visual fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Font = "gothic"
	o.FontSize = 12
	o.Top = 58
	o.Left = 23
	o.Right = 23
	o.Bottom = 35
	o.BackgroundImage = 1
	o.NumberFormat = "none"
	o.NumberAlign = "center"
	o.TextColor = "#3e2b22"
	doc := Document{Title: "Архив Адептус Механикус", Options: &o, Blocks: []Block{{Type: BlockMarkdown, Markdown: "## Проверка фона и полей\n\n" + strings.Repeat("Данные сохранены в архиве. Текст располагается внутри орнамента, не перекрывая эмблему и рамку.\n\n", 30)}}}
	var out bytes.Buffer
	if err := Render(context.Background(), doc, func(int64) ([]byte, error) { return data, nil }, &out); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/quests-parchment.pdf", out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
