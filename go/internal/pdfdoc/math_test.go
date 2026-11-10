package pdfdoc

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

const mathSample = `## Среднегодовая стоимость ОПФ

> Формулы из заметки №137, рядом с обычным текстом.

$$\Phi_{ср} = \Phi_{н} + \frac{\sum \Phi_{вв} \cdot t}{12} - \frac{\sum \Phi_{выб} \cdot t}{12}$$

- Ввод: $75\cdot10 + 50\cdot8 + 39\cdot4 + 18\cdot1 = 1324$
- Вывод: $3\cdot10 + 4\cdot8 + 7\cdot4 + 15\cdot1 = 105$

$$\Phi_{ср} = 8825 + \frac{1324}{12} - \frac{105}{12} = 8825 + 110{,}333 - 8{,}75 = \mathbf{8926{,}58}\ \text{тыс. руб.}$$

**Фондоотдача** $= \dfrac{4384}{8926{,}58} = \mathbf{0{,}491}$

**Фондоёмкость** $= \dfrac{8926{,}58}{4384} = \mathbf{2{,}036}$

$$29\,552 + 4\,622 + 652 + 1\,969 = 36\,795\ \text{тыс. руб.} = \mathbf{43{,}32\%}$$
`

func TestMathNote137(t *testing.T) {
	for _, expr := range []string{`\Phi_{ср} = \Phi_{н} + \frac{\sum \Phi_{вв} \cdot t}{12}`, `\mathbf{8926{,}58}\ \text{тыс. руб.}`, `\dfrac{4384}{8926{,}58}`, `43{,}32\%`} {
		if _, err := parseMath(expr, 10); err != nil {
			t.Errorf("%s: %v", expr, err)
		}
	}
	var b bytes.Buffer
	if err := Render(context.Background(), Document{Title: "Markdown и формулы", Blocks: []Block{{Type: BlockMarkdown, Markdown: mathSample}}}, nil, &b); err != nil {
		t.Fatal(err)
	}
	if p := os.Getenv("PDFDOC_MATH_OUT"); p != "" {
		if err := os.WriteFile(p, b.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMathInvalidAndBounded(t *testing.T) {
	for _, expr := range []string{`\unknown{x}`, `\frac{`, strings.Repeat("{", 101)} {
		if _, err := parseMath(expr, 10); err == nil {
			t.Errorf("accepted %q", expr)
		}
	}
	render(t, Document{Blocks: []Block{{Type: BlockMarkdown, Markdown: "Ошибка $\\unknown{x}$; код `a_b`; цена $10 и $20."}}})
}

func TestMathCommandsAndErrors(t *testing.T) {
	ok := []string{
		`\sqrt{x}`, `\mathrm{d}x`, `\operatorname{tr}A`, `\mathit{abc}`,
		`\left(\frac{a}{b}\right)`, `a\quad b\qquad c`, `a\,b\;c\:d\!e`,
		`\% \{\} \_ \$ \# \&`, `\alpha+\beta\leq\gamma`, `x_{i}^{2}`, `x^2_i`,
		`\text{тыс. руб.}`, `a = b \pm c \times d \div e \cdot f`,
		`\sum_{i=1}^{n} i`, `{a+b}`,
	}
	for _, expr := range ok {
		box, err := parseMath(expr, 10)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if box.w <= 0 && expr != `a\,b\;c\:d\!e` {
			t.Errorf("%s: empty box", expr)
		}
	}
	bad := []string{
		`a_{1}_{2}`, `a^{1}^{2}`, `}`, `_x`, `^x`, `\`, `\?`,
		`\text`, `\text{a{b}`, `\text{a\b}`, `{a`, `a}`, strings.Repeat("a", 4097),
		`\frac{1}`,
	}
	for _, expr := range bad {
		if _, err := parseMath(expr, 10); err == nil {
			t.Errorf("accepted %q", expr)
		}
	}
	deep := strings.Repeat("{", 25) + "x" + strings.Repeat("}", 25)
	if _, err := parseMath(deep, 10); err == nil {
		t.Error("accepted too-deep group")
	}
}

func TestMathMarkdownBlocks(t *testing.T) {
	md := "строка $$\\alpha+\\beta$$\n\n$$\n\\frac{1}{2}\n$$\n\n$$\nбез закрытия\n"
	out := render(t, Document{Blocks: []Block{{Type: BlockMarkdown, Markdown: md}}})
	if len(out) < 1000 {
		t.Fatalf("small PDF: %d", len(out))
	}
	var many strings.Builder
	for i := 0; i < 201; i++ {
		many.WriteString("$x$ ")
	}
	render(t, Document{Blocks: []Block{{Type: BlockMarkdown, Markdown: many.String()}}})
}
