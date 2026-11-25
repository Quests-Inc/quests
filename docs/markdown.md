# Markdown in Quests

The same sanitized renderer is used for notes and descriptions. UI explanations in forms are separate from user prose
(see [ui-style.md](ui-style.md)). Related work: quest=523, quest=524, quest=525, quest=526, quest=527.

## Static syntax

| Element | Syntax | Browser | PDF |
|---|---|---|---|
| Headings | `#` through `######` | Six styled levels | Six levels |
| Emphasis | `**bold**`, `*italic*`, `~~deleted~~` | Yes | Yes |
| Lists | `-`, `1.`, nested lists | Yes | Yes |
| Task list | `- [ ]`, `- [x]` | Tickable scratch marks; «Перенести в текст» bakes them into the source | Printed check marks of the saved text |
| Collapsible block | `<details>` / `<summary>Заголовок</summary>` | Real collapsible plate, closed unless `<details open>` | Always expanded; the summary becomes a bold lead line |
| Diagram | ```` ```mermaid ```` fence | Rendered to a sanitized SVG, lazy-loaded | Source as a code block with an explicit «диаграмма не рендерится в PDF» marker |
| Explanation / quote | `> text`, `>` between paragraphs | Left border and muted text, no rounded plate | Left border |
| Code | Backticks / fenced blocks | Highlighting, language label, copy | Highlighting and wrapping |
| Table | Pipe table, column alignment | Local horizontal scroll when needed | Wrapped cells |
| Rule | `---` | Themed line | Printed line |
| Link | `[name](https://…)`, `note=N` etc. | Entity navigation or safe external link | Text and external links |
| Image | `![caption](attachment=N)` | Attachment images only | Authorized attachments only |
| Math | `$inline$`, `$$display$$` | KaTeX, local fonts | Native vector layout, embedded DejaVu fonts |

Soft newlines remain line breaks. Raw HTML is restricted; it is not an extension API. External images are not loaded.
Of the interactive extensions researched in quest=528, exactly three were added: collapsible blocks, browser-only
mermaid diagrams, and task ticks that are scratch state until they are explicitly transferred into the note text.
Task markers inside code and math never count; a transfer rewrites `- [ ]` ↔ `- [x]` positionally and refuses when the
note changed under the ticks. Mermaid renders with `securityLevel: 'strict'`, root `htmlLabels: false` and a DOMPurify
SVG pass; a failed render keeps the source with a visible note instead of breaking the document.

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

## A piece of a note (`span-id`)

`<!-- span-id: light -->` … `<!-- /span-id: light -->` names an arbitrary piece. The key uses the same grammar as
`table-id` and `row-id`. The closing comment repeats the key. A key is unique in the note. A missing close, a close
with another key, or two pairs that cross is not a target. The pair may sit on its own lines around blocks, or inside
a paragraph around a phrase. In preview the comments stay hidden. Inside a fenced code block they stay code.

`note=544#light` is one link to that piece. `note=540#spell/classes` is one link to a table row. For backlinks both are
still a link to the note. A click opens the note and scrolls to the piece. The link text is the piece itself, as
readable text, without the source note's title and without quotation marks. A missing piece stays readable and is
marked «Кусок недоступен»; a missing or broken row is marked «Строка недоступна».

In the text and split editors, service comments are coloured and the source is unchanged: table orientation, `table-id`,
`row-id`, a `span-id` pair, and any other `<!-- … -->` each have their own colour. Fenced code is not recoloured.

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

## Note PDF books

Both note export menus offer PDF → «Только эта заметка» and
«Сшить вложенные заметки в книгу». A book includes the selected note and every
accessible descendant, independent of current search or collapsed tree.
Sibling order matches the notes tree (children first, pinned, sort_order, id).
References do not add other notes. Full bodies are loaded before export;
a failed read aborts instead of silently omitting a chapter. The open root uses
its editor text, including unsaved edits; children use saved text.

Each note is baked in its own variable/script context once. Images, tables,
code and math use the existing renderer. Title and clickable contents precede
the text; direct children start on new pages, deeper descendants form sections.
Contents have page numbers; PDF bookmarks follow the note hierarchy. A matching
first Markdown heading is omitted to avoid repeating the card title. Included
note references link to chapters; span and row markers provide local targets.
Other labelled references link to Quests. Existing PDF limits apply, plus a
1000-note ceiling.

## PDF export settings and presets

Quick actions export immediately with the browser's selected quick profile.
«Настроить экспорт…» opens the shared settings dialog for notes, books and quests;
its composition selector can switch a note export into a book. Merely adjusting
or saving a preset does not change quick export: the explicit quick-export
checkbox applies the profile when a PDF is downloaded. Five built-in profiles
(Standard, Compact, Reading, Print, Book A5) and up to 20 custom presets are stored
in localStorage by username and workspace ID. They are local browser preferences,
not server settings or workspace-export contents.

Document `options` support page sides (80–600 mm), orientation, four margins and
binding (0–100 mm), font (serif/sans/mono/gothic), proportional font size (6–24 pt),
line spacing (1–2.5), text and background RGB colours, page number format/position/
alignment, chapter breaks and mirrored margins. Content must retain at least
40 × 40 mm. A visible page number needs at least a 10 mm margin on its side.
Omitted options retain the historical defaults; invalid values return HTTP 422.

A compact plus beside the background selector opens image upload. Uploads become attachments
of the exported note (the root note for a book), or of the exported quest. The
dialog explains the destination before upload and selects the new background
automatically; uploading does not change quick-export preferences.

Backgrounds use a workspace attachment ID and the same authorized raster loader
as inline images, once per document. The prepared image repeats on every page,
with opacity 0–1 and contain/cover fitting. An inaccessible requested background
fails explicitly instead of silently generating an unframed document.
CyrillicGothSHA regular and bold faces are embedded in the server binary; symbols
outside their character coverage fall back to DejaVu. Code remains monospaced.

The explicit preview action renders the real PDF, then displays its pages using
a locally bundled PDF.js worker. Downloading an unchanged preview reuses its Blob
and does not rerun variable/script baking. Changing typography or composition
marks the preview stale. The dialog and page preview scroll independently on a
wide display; a narrow display uses one settings/preview column.
