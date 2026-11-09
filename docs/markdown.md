# Markdown in Quests

The same sanitized renderer is used for notes and descriptions. UI explanations in forms are separate from user prose
(see [ui-style.md](ui-style.md)). Related work: quest=523, quest=524, quest=525, quest=526, quest=527.

## Static syntax

| Element | Syntax | Browser | PDF |
|---|---|---|---|
| Headings | `#` through `######` | Six styled levels | Six levels |
| Emphasis | `**bold**`, `*italic*`, `~~deleted~~` | Yes | Yes |
| Lists | `-`, `1.`, nested lists | Yes | Yes |
| Task list | `- [ ]`, `- [x]` | Disabled, read-only checkboxes | Printed check marks |
| Explanation / quote | `> text`, `>` between paragraphs | Left border and muted text, no rounded plate | Left border |
| Code | Backticks / fenced blocks | Highlighting, language label, copy | Highlighting and wrapping |
| Table | Pipe table, column alignment | Local horizontal scroll when needed | Wrapped cells |
| Rule | `---` | Themed line | Printed line |
| Link | `[name](https://…)`, `note=N` etc. | Entity navigation or safe external link | Text and external links |
| Image | `![caption](attachment=N)` | Attachment images only | Authorized attachments only |
| Math | `$inline$`, `$$display$$` | KaTeX, local fonts | Native vector layout, embedded DejaVu fonts |

Soft newlines remain line breaks. Raw HTML is restricted; it is not an extension API. External images are not loaded.
No new interactive Markdown syntax is added in this pass. Further research lives in quest=528.

## Quotes are prose, not UI panels

```md
## A section

> A short explanation of this section.
>
> A second paragraph with **emphasis** or a list.
```

Quotes have a left border, inset text and the theme's muted foreground. They do not inherit the rounded backgrounds of
quest descriptions or `.explain` UI panels. Nesting, lists and multiple paragraphs work inside them.

## Mathematics

```md
The ratio is $\frac{4384}{8926{,}58}$.

$$
\Phi_{ср} = \Phi_{н} + \frac{\sum \Phi_{вв} \cdot t}{12}
$$
```

Inline delimiters must touch the expression: `$x$`, not `$ x $`. Escape a literal dollar as `\$`; `$10 and $20` stays
currency. Math inside code stays code. A display formula has its own horizontal scrolling area in a narrow view.
There is no click action on a formula: its source remains in the note editor.

Browser math runs with `trust: false`, no external resources or shared macros, at most 200 macro expansions, 4096 source
characters per formula, and 200 formulas per rendered text. Invalid math keeps the escaped source with a tooltip.

PDF math is deliberately a smaller subset: numbers and Unicode letters, groups, subscripts/superscripts, fractions
(`\frac`, `\dfrac`, `\tfrac`), square roots, bold/roman/italic, `\text{…}`, common Greek letters, operators and symbols.
It handles the formulas from note=137. Long expressions scale to the current column and formulas stay together across
page breaks. Very long expressions may become small; split them into several display formulas for print.
Unsupported commands (for example environments such as `aligned`/matrices) print their original source with an explicit
“формула не поддерживается в PDF” marker. They are never silently omitted. Limit: 4096 characters, 100 groups, 24 levels
of nesting, 200 formulas per document. Both browser and PDF engines have regression coverage for the shared subset.

## Stable table targets for script results

See [embeds.md](embeds.md#table-result-targets). The prototype targets rows, not columns/cells. Keys are manually assigned
lowercase Latin identifiers (`a-z`, digits, `_`, `-`, up to 64 characters, starting with a letter).

```md
<!-- table-id: diagnostics -->
| State | Action |
|---|---|
| <!-- row-id: ok -->Services respond | Continue |
| <!-- row-id: restart -->Service is unavailable | Restart and inspect logs |
```

Metadata is invisible in the browser and PDF. Reordering rows preserves identity. Table IDs must be unique within the
rendered text, row IDs within that table. Missing or duplicate targets leave the result readable and show “Строка
недоступна”; there is no substring matching or positional fallback.

## UI review checklist

Check a populated view and an empty view, loading/error, hover, keyboard focus, disabled/busy, long titles and content.
Use both a dark and a light theme, a narrow container/phone and a wide viewport. Ensure the page scrolls once, and only
wide tables/code/formulas get local horizontal scroll. A standalone tab has page navigation and no modal close cross.
Repeated controls must use the same labels and visible action menus, including on touch devices.

## Editor references and script insertion

Type `@` to open the available references immediately, then type a title to narrow them down. Entity-kind labels reuse
the graph's shared palette (`shared/lib/entityColors.ts`). Only scripts with the purpose “Вставки в текст” appear as
script insertions. Choosing one opens its insertion dialog with that script selected; confirmation replaces the `@query`
with `script=N{…}` and its chosen flags. Cancelling keeps the original text. There is no separate script-insertion button
inside the text field. Regular entity choices still insert their usual `kind=N` references.
