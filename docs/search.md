# Searching notes

`GET /api/notes/search?q=…&limit=…` (MCP: `search_notes`). In the interface it is the «Поиск» (Search) field on the «Заметки» (Notes) tab
(from two characters: a ranked list with snippets; until the answer arrives the old filter over the tree stays).

## What it can do (PostgreSQL)

- **Word forms, Russian and English:** “квесты / квестов”, “deploy / deploys” share a stem (`tsvector` of both
  configurations, column `note.search_tsv`, a GIN index, migration `d1f3a5c7e9b2`).
- **Relevance, not a substring:** a title weighs more than the text (`ts_rank_cd`), and results are ordered by rank.
- **Prefix of the last word:** “депл” → “деплой”; not applied to a word with `-`, in quotes or followed by a space.
- **Keyboard layout:** “ltgkjq” is also searched as “деплой” (and the other way round).
- **Typos in the title:** `pg_trgm` (`word_similarity > 0.45`); such results come after real ones (`where: fuzzy`).
- **Operators:** `"exact phrase"`, `-exclude`, `or`.
- **Snippets:** a fragment with the found words in `⟪ ⟫` (the client escapes the text first, then highlights).
- **Explainability:** every result says where it matched: `title` | `body` | `fuzzy`.
- **Privacy:** notes flagged `PRIVATE` are never given to agents (`X-Quests-Source: mcp`); workspaces are isolated by RLS.

If PostgreSQL search is unavailable (no extension, a schema in the middle of a migration), the query falls back to a simplified in-memory
count: words in the title and text, layout, snippets; no morphology and no typos.

## Filters in search fields (quest=371)

In the interface's search fields, `key:value` tokens are written next to the words, and `-key:value` means “not”. `flag:` is only for
real flags; everything else is a widget's own keys. Several values of one key mean “or” (`status:done status:failed`),
for `flag:` they mean “and”. While `key:` is being typed, hints appear under the field; an unknown value gives a warning.
The query string is the single source of truth: the switches next to the field (segments, dropdowns, a checkbox, a
calendar) read their value from the tokens and write it back, so the field and the switches always agree.
Parsing is shared: `frontend/src/shared/lib/searchFlags.ts`, hints are `shared/ui/FlagHints.svelte`; each widget has its own vocabulary.

| Where | Tokens |
|---|---|
| Notes | `flag:outdated` (`stale`) / `readme` / `private` / `category` / `pinned` / `check`; the server search and MCP `search_notes(flags="-outdated")` take the same flags |
| Journal (quests) | `status:active\|done\|failed\|expired\|frozen\|archived\|all` (`all` = the “show completed” checkbox; any `status:` shows closed ones), `flag:pinned\|hidden` (in the overlay always / never), `type:automated\|manual`, `sig:epic`, `tag:infra`, `cat:<category>` and `line:<questline id>` (= the dropdowns of categories / questline) |
| Attachments | `owner:quest\|questline\|note` (segment), `flag:hidden` (“hidden”), `show:all` (“all”), `ftype:pdf` (an extension, a MIME type or part of it: `png`, `image`), `rev:N` / `>N` / `<N` / `>=N` / `<=N` (the number of versions) |
| History | `type:quest\|note\|attachment`, `act:done\|new\|edit\|miss\|del`, `line:<id>`, `cat:<category>`, `tag:<tag>`, `when:today\|7\|30`, `day:YYYY-MM-DD`: all the page's selectors; the address bar keeps only `q=` (the old `?et=&kg=…` turn into tokens) |
| Map centre, “find an entity” | `type:quest\|note\|questline`, `flag:outdated\|pinned`, `status:active\|done\|failed\|frozen` |

The flag filter over the ranked notes results works at the candidate level (up to 50); for MCP and
`GET /api/notes/search?flags=outdated,-private` it is done on the server.

## Searching inside a note

`GET /api/notes/{id}/search?q=…` → `{"terms": [...], "passages": N}`. The same rules (word forms ru/en, prefix, layout,
“phrases”, `-exclusions`), but the note's text is cut into paragraphs and each is checked on its own; the answer carries the words as they are written
in the text. In the interface: the magnifier button or Ctrl+F in an open note, a highlight in the preview (CSS Highlight API),
Enter / Shift+Enter for next / previous. In the “source” mode there is no preview, so there is nothing to highlight.

## Known limits

Snowball stems are coarse: “приглашения” will not find “приглашаются”. A note's body goes into the index up to 300,000 characters.
Typos are caught only in the title. Semantic search (embeddings) was not built.

## Quality check

`tools/search_eval.py` creates a fixed set of notes and 15 queries (word forms, prefix, layout, a typo,
a phrase, an exclusion, English, an empty result) and prints the rank of the expected note and a “rank 1 / top-3” total.
Run it in an empty workspace on a copy: `python3 tools/search_eval.py --base … --user …`.

## What gets read more: a ranking hint (quest=353)

Reading signals accumulate in `note_stat` (migration `e3a5c7e9b1d4`) as **aggregates, without a log of who read what and when**,
per workspace, deleted in cascade with the note.

| Signal | Weight | Note |
|---|---|---|
| Opened in the web interface (`X-Quests-Source: web`) | 1 | a repeat within 10 minutes does not count |
| Read by an agent (`mcp`, `get_note_context`) | 0.3 | its own dedup window; private ones do not count |
| “Useful” (`rate_note`, `POST /api/notes/{id}/feedback`) | +2 | the strongest explicit signal |
| “Not useful / misleading” | −3 | the score does not go below zero |
| Shown to an agent in results | 0 | counted separately (`shown`), adds no interest |

Background clients (the CLI, bot, HUD, export, graph) do not count as reads.

**How it affects results.** The score halves every 30 days (lazily, on read), is squeezed by a logarithm and only
adjusts relevance: the result = relevance × (1 + adjustment), where the adjustment is ≤ +30 %: up to +20 % for reading, up to +10 %
for a “new note” (the bonus fades in two weeks, so fresh and rare things do not sink), a manual weight of −2…+2 at 5 % each. Typo
matches always stay behind real ones. This way an accumulated score does not push out relevance and does not lock in a random start.

**Explainability and correctability.** In the results and in `list_notes(sort="popular")` a note has a `popularity` block:
the decayed score, the numbers of opens and shows, the ratings, the adjustment and the reason (“recently read”, “new note”,
“manual weight”…). Reset one note: `DELETE /api/notes/{id}/stats`; set a manual weight: `PUT /api/notes/{id}/weight`.
