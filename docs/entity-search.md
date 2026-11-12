# Entity search

`POST /api/search` is the shared search for lists, entity pickers, map roots and `@` mentions. The legacy notes-search endpoint uses the same matcher and keeps its response format and reading-history metadata.

```json
{"q":"note=378","types":["note"],"filters":[{"key":"flag","values":["pinned"],"not":false}],"limit":30,"offset":0}
```

Results contain `hits` (kind, id, title, where, snippet, score, optional parent kind/id), `more` and `next` (the next offset). A page is at most 100 hits; a list can consume subsequent pages. An explicit empty types list means no types. Omitting types selects the ordinary document/entity kinds.

The matcher and response contract live in `platform/search`. Each context supplies its own narrow SQL projection in its `search.go`; `store.SearchEntities` federates them without moving another context's SQL into it. All queries go through the usual workspace/RLS driver. MCP searches exclude private notes and attachments owned by private notes.

Types: quest, step, questline, note, attachment, template, script, hook, event; explicitly requested catalogs also support tag, category, runner and job. `kind=N` and `kind:N` are strict identity searches. A bare positive number retains text matches, but an exact ID ranks first. Numeric substrings of other IDs do not match identity. An existing query's parsed widget filters, or an explicit allowed/excluded candidate scope, apply before ranking and LIMIT. A filter can optionally name `kind` when it applies to only one type (for example embed-only scripts in mixed `@` suggestions). An empty allowed list means an empty scope, never an unrestricted search. Scopes can narrow access, never widen RLS.

Matching order is exact identity, title (substring or Russian/English stem/prefix), body, title typo. Keyboard-layout variants and the same PostgreSQL matcher are used for every type. Notes retain their bounded reading-history/newness/manual multiplier and outdated demotion inside these tiers; it is applied by the provider before LIMIT for both legacy and shared clients. Remaining ties are deterministic by title, kind and ID. The first version computes text vectors from context projections; it does not maintain a second copy of every document. Separate indexes can be added after measuring the queries. The existing notes index remains available to other note operations.

Widget-specific constraints such as legal move destinations, currently enabled templates, or selected ownership filters can be expressed as allowed IDs from the complete catalog. This is intentional scope selection, not local text matching. Standard note flags are evaluated by the provider, before the page limit. Results from steps carry the parent quest so the journal can show the owning task.

The browser uses one abortable, debounced search lifecycle. A new query clears selectable stale results; pending/error states are explicit, with no local search algorithm silently replacing server output. Small static option sets still use local filtering. In-document text search and historical-record search keep their own result types; history also understands subject IDs.

The shell asks for `GET /api/notes?catalog=1`: descriptions are previews of at most 320 characters, marked `description_complete:false`. Opening/editing/exporting uses the full detail endpoint. External edits of an open note fetch its full document before comparing or applying changes. Search always reads the full stored text. Quest catalogs retain full documents because the current journal detail/step editor consumes them; making those brief requires migrating the detail loader as well, rather than silently truncating editable text.
