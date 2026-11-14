# Server limits

Host administrators configure operational budgets through `quests-server limits`.
Workspace owners cannot change them: there is no HTTP/MCP write endpoint. The CLI
requires access to the server's data directory (`QUESTS_DATA_DIR`) and uses the same
configuration as the API. Mount that directory persistently and restrict its host permissions.

```
quests-server limits list
quests-server limits set git_bytes 4294967296
quests-server limits set values_effective 128
quests-server limits set values_own 64
quests-server limits set values_bytes 16384
quests-server limits reset git_bytes
```

`list` is JSON: effective value, source (`default`, `saved`, `env`), unit, default,
safe min/max and `restart_required`. Values are integers in the displayed unit;
bytes use binary sizes (1 GiB = 1073741824). Unknown names, bad integers,
out-of-range budgets and effective-name limits below the own-entry limit are refused.
Set `values_effective` before raising `values_own` beyond it.

Precedence: existing environment variable > saved setting > built-in default.
`reset` removes the saved override; it does not unset the environment. Numeric
egress overrides preserve the existing `QUESTS_EGRESS_*` variable names and units.
Host/port/CIDR/proxy allowlists, egress mode and per-workspace grants retain their
security policy; a numerical budget never enables network access.

The server-owned `server-limits.json` is replaced atomically; an advisory file lock
serializes CLI writers. `.audit.jsonl` records time, OS uid, previous override and
requested value/reset before publication (an unsuccessful rename can therefore
leave an attempted edit in the log). No secret values are recorded. Files are
created with mode 0600. Keep these files with the persistent data-directory backup.
A corrupt configuration refuses startup; later read failures use conservative minima.

## Inventory and application

| Keys | Scope / source | Application |
|---|---|---|
| `git_bytes` | per-workspace Git storage; `repository/handler.go`, `wsarchive/repositories.go` | live, shared push/clone/import checks; per-workspace repository lock |
| `attachment_bytes`, `upload_bytes` | per-workspace storage / per-file; `knowledge/http`, `graphdraft`, `wsarchive` | live upload/revision/import checks; existing workspace quota checks retained |
| `values_own`, `values_bytes` | per owner; `execution/values_upsert.go`, `values_batch.go` | live, final own set, owner lock and transaction; manual writes may replace literal/formula |
| `values_effective`, `values_chain` | final inherited environment / note ancestry; `execution/values.go` | live admission budgets; existing reads are preserved up to hard safety bounds (4096 names, 256 parents), with `truncated` at the safety bound |
| `embed_auto`, `foreign_values` | one rendered/exported text; `shared/lib/serverLimits.ts` | client fetches actual server capabilities on app startup; refresh the page after changing these budgets |
| `graph_nodes`, `build_bytes` | stored graph / builder template and output; `execution/graph` | live validation; final embedded response retains its independent 64 KiB safety ceiling |
| `egress_body`, `egress_timeout`, `egress_rate`, `egress_repository_rate`, `egress_concurrency` | outbound transport; `platform/egress` | **restart required**: existing pools, semaphores and timeouts retain startup values until restart |

The authenticated `GET /api/capabilities` returns applied numeric limits, including
startup values for restart-required settings. Other live settings are reread when
used. Repository and attachment APIs retain their actual quota fields used by UI.
UI/export use the same automatic-insert and foreign-variable budgets. Skipped auto
inserts remain clickable and explain the limit; excess foreign references show a warning.

Lowering storage quotas never deletes existing data. Reads remain available;
new uploads/pushes/imports must fit the new budget. Existing own-variable sets
above a lowered limit may be edited without increasing their count/byte use;
new copies must fit current limits. Inherited environments are checked after the
whole manual package is merged. Automatic `sink.values` still refuses to replace
formulas. Reducing graph/builder budgets affects later validation, not stored rows.

## Fixed language and safety rules

Not administrator quotas: value name grammar / reserved environment names (24 ASCII
characters), ±1e9 value range, string size (500 **UTF-8 bytes**, not Unicode characters),
formula size (1024 bytes), round precision (0–9), DAG/cycle rules, roll/evaluation
budgets, JSON nesting and hard request/document safety ceilings. These protect
language semantics and execution cost; raising owner quotas does not disable them.
Single-value request body remains 16 KiB; the manual bulk route has a separate
256 KiB ceiling and the final owner byte budget still applies. Large files and
archive/table limits remain independent and are not represented as unlimited quotas.
