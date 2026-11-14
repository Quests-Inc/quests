# Reading table data and building script results

Server nodes for embedded scripts; no runner, arbitrary code or data copied into the graph is needed.

## Reading a note

`source.note` exposes `title` and `description` (raw Markdown). In an embedded evaluation, an explicit `params.id`
reads that note in the current workspace. Without an ID it reads the note whose text launched the script, supplied by
`host: {kind: "note", id: N}` to `POST /api/scripts/N/eval`. A caller without a current note must specify an ID.
Embedded sources expose neither secrets nor write operations. They do not execute the scripts inside the source text,
so reading a note that embeds the reading script does not recurse. Private notes cannot be read by embedded sources;
the new description port also omits private bodies from ordinary run snapshots. Existing title/ref/secrets ports for
ordinary bound scripts are unchanged.

## Markdown: table (`markdown.table`)

Input `text: str`; optional `column: int`. Parameters:

```json
{"table":"choices","column":0,"omit":[]}
```

Uses Goldmark's GFM table parser. `table` names a `<!-- table-id: choices -->` comment immediately before a Markdown
table; each body row must contain exactly one unique `<!-- row-id: stable-key -->` comment in a cell. Keys follow the
renderer's lowercase ASCII letter / digits / underscore / dash contract (1–64 characters). Table ID must identify one
table. Missing or duplicate IDs, empty tables and invalid columns fail the run with an explanation.

Outputs:

- `value: doc`: `{id, headers: [text], rows: [{id, cells: [text]}]}`.
- `rows: doc`: the array of complete rows, for `random` with `kind: "pick"`.
- `items: doc`: the same array when column is zero; otherwise `[{id, value: cellText}]` for the chosen column.

Column numbers start at 1, including the first displayed column; zero means complete rows. A wired `column` input
wins over the static parameter. `omit` explicitly excludes exact cell texts from the selected column; no placeholder
such as `—` or an empty string is implicitly treated as absent. An empty eligible set is an error. `omit` with whole-row
mode is an error. Text is taken from parsed inline nodes: emphasis/link markup and metadata are removed; scripts remain
plain source text and are not evaluated. Initial support is for ordinary GFM tables, not raw HTML tables.

Limits: 64 KiB source and each output, 1000 body rows, column 0–100. The browser uses `marked`, while the server uses
Goldmark: their table/marker contract is exercised on the same shape of GFM fixtures; full Markdown dialect parity is
not claimed. Escaped pipes, inline formatting and stable markers are covered by parser tests.

## Text: build (`text.build`)

```json
{"inputs":[{"name":"choice","type":"str"}],"template":"Chosen: {{choice}}"}
```

Each named input creates a required port; types are `str`, `int`, `float`, `bool`, `doc`. `{{name}}` inserts text.
A document string becomes its text; other document values use their JSON representation. There is no evaluation,
formatting language or implicit Markdown/HTML rendering. Output is `value: str`.

## JSON: build (`json.build`)

```json
{
  "inputs":[{"name":"row","type":"str"}],
  "template":{
    "kind":"markdown-target","version":1,"text":"Selected row",
    "target":{"table":"choices","row":{"$input":"row"}}
  }
}
```

A whole object `{"$input":"name"}` inserts the typed value. Every other tree element is literal. Strings are escaped
by JSON serialization, documents are inserted as complete values, and `null`, `false`, zero, arrays and objects retain
their type. `$input` is reserved for substitutions: it cannot share its object with other keys. No interpolation inside
JSON strings or keys. Output `value: doc` connects directly to Result; an intervening serializer is unnecessary.

Both builders allow up to 16 inputs and 64 KiB templates/results; JSON template nesting is at most 32 levels. Invalid
parameters are rejected when validating the graph. Runtime data errors fail the run. These nodes add no generic type
conversion, dynamic JSON path, loops or user code.

## Result and empty arguments

`sink.result` accepts a scalar or document. Strings preserve the original response contract; other values are serialized
to JSON, still returned as the API's string `value`. `markdown-target` v1 therefore needs no client envelope change.
A graph with no arguments can be inserted as `script=N{}`; the reader sees “Запустить”. A click runs it, opening the note
does not (unless `?auto` is explicitly present).

## Parameterized column generator

One graph can serve every column:

`source.note.description → markdown.table.text → random.pick → json.get(value, str) → Result`

`Args(column: int) → markdown.table.column`

For a table whose first column is a row number, `script=N{2}?hide` selects from the first data column,
`script=N{3}?hide` from the next, and so on. Multiple inserts retain independent results even when they share a script.
`omit: ["—"]` handles a five-item column beside six-item columns. Neither the prose nor the option lists are duplicated
in the graph. Table ID remains a parameter of the node; a fully dynamic table selector is outside this initial scope.

For row navigation, instead choose from `rows`, extract `id`, feed it into the JSON builder shown above, and connect its
output to Result. This selects a whole row. Independent column choices do not imply cell highlighting, which v1 does
not support.

## Typed constants and explicit conversion

`constant` is a source with `{"type":"int","value":42}` and output `value`.
Supported types: str/int/float/bool/doc, including JSON null only for doc. The JSON
value must match its type; an int keeps signed 64-bit precision. It executes no code.

`convert` has input `in`, output `value`, params `from`, `to`, optional `mode` and
`round`. The value mode accepts strict scalar conversions: booleans parse only the
strings `true`/`false`; integer strings are base-10 signed integers; numeric strings
must be finite. Integers outside ±2^53 cannot become float silently. Float → int
requires an integral in-range value unless `round` explicitly selects floor, ceil,
trunc or nearest (ties away from zero). The default `exact` refuses fractions and
overflow. Unsupported conversions fail the run.

Document scalars retain numeric spelling/precision. `mode: "parse_json"` is only
str → doc and validates the JSON string; `mode: "stringify"` is only doc → str and
serializes compact JSON. These are distinct from interpreting a document's scalar.

`json.get` now accepts optional `path: str`; its wired value takes priority over
params.path. Numeric segments select array indices, e.g. `items.2.name`. Optional `index: int` appends a nonnegative array index to the selected path (static or wired), e.g. path `items` and index 2 select `items.2`.
`markdown.table` similarly accepts optional `table: str`, overriding params.table.
Static parameters remain required fallbacks and preserve old graphs. Column input
priority remains unchanged. Neither operation evaluates source text.
