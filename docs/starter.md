# Starter examples

The Russian set in `go/internal/store/starter/ru.json` is embedded into the server. It contains a tutorial questline,
quest and steps, a category with operational guides, readable variables and an embedded dice script, one script per
purpose, a disabled hook and a disabled weekly template. Runner scripts explicitly explain that a runner and local
configuration are required. No network request or user command runs just because the set was installed.

## Entry points

- `quests-server useradd`: the first personal workspace receives examples. Claiming a nonempty legacy default
  workspace preserves it without adding examples. Creating later workspaces or starting/deploying the server does not seed.
- **Настройки → Примеры и гайды → Добавить примеры** seeds the current workspace (owner only).
- Host administrator: `quests-server workspace seed ID` and `workspace seed-remove ID`.
- `POST /api/workspace/starter`, `DELETE /api/workspace/starter` (owner only). Results report added, updated, preserved
  and deleted entries. The data view refreshes after installing or removing a set.

## Identity, updates and user edits

A stable `_key` such as `starter/notes/runner` identifies an example independently of its title or entity number.
`seed_item` records workspace, key, entity table/id, set version and two hashes. `content_hash` hashes the source before
IDs and references are remapped; `applied_hash` hashes the installed editable row (and a script's current graph), excluding
identity, generated search data and creation/update/deletion metadata. Comparing the installed hash detects edits to any
field, including fields absent from the original set. The source hash does not vary between workspaces.

- New key: insert a new example with fresh IDs.
- Existing unchanged source: do nothing.
- New source version and unchanged installed row: update in place. A script gets a new version; older graphs remain intact.
- User-edited row: preserve it. There is no automatic overwrite or merge.
- Deleted, trashed or explicitly removed entry: remember the key and never resurrect it automatically.

The existing workspace import handles IDs, foreign keys, graph sources and markdown references. It now rewrites only
rows written by that import, so installing examples into a populated workspace cannot rewrite unrelated content.
Hooks and script bindings are included in workspace archives too. Seeding and its bookkeeping share one transaction
and a workspace lock; failure rolls everything back and parallel requests cannot duplicate the set.

## Removing and restoring

«Убрать примеры» uses the seed records, not a title search. Untouched notes, quests, scripts, hooks, templates and the
questline go to the normal 30-day trash. User-edited examples stay; edited or user-added children protect their seeded
owners. Values and steps stay with the owning entity. Seed identity remains after removal, including after a purge.

Templates and questlines now support the same trash lifecycle. Their existing quest links survive until permanent purge;
a restored template is disabled until an owner enables it. Purge detaches independent quests and then removes the
container's rows, secrets, versions/children where appropriate, attachments and custom icon. The retention worker handles
these containers too. The MCP `trash` tool lists and restores both kinds.

## Validation

PostgreSQL tests cover repeat install, source upgrades, preserving edits, remapped links, graph validation, removal,
no resurrection, old script versions, container restore/purge and retention. The frontend covers the existing navigation,
completion syntax and secret permissions. The seed contains no credentials and no automatic external calls.
