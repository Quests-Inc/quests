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
