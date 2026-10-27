# Trash

Deleting a quest, a note, a script or a hook (the web UI, `DELETE /api/quests/{id}`, `DELETE /api/notes/{id}`,
`DELETE /api/scripts/{id}`, `DELETE /api/hooks/{id}`, the CLI, the bot, MCP `delete_note`, `script` delete, `hook` delete)
**moves it to the trash**; nothing is erased. Items stay for 30 days, then the scheduler erases them
(`httpapi.PurgeExpiredTrash`, hourly).

| | |
|---|---|
| Web | Settings → Корзина: list, restore, erase one, empty |
| API | `GET /api/trash/{quests,notes,scripts,hooks}`; `POST /api/trash/{kind}s/{id}/restore`; `DELETE /api/trash/{kind}s/{id}` (erase for good, attachments too); `DELETE /api/trash/{kind}s` (empty). Scripts and hooks: restore and erase are owner-only |
| MCP | `trash` — `action: list` or `restore` (`kind` = quest, note, script or hook; `id`) |

## What comes back

- **Quest**: with its steps, tags, secrets, attachments and history, as it was. A questline deleted meanwhile is dropped from
  it; a template period that was emitted again meanwhile detaches the restored quest from the template.
- **Note**: under its parent (at the root if the parent is gone or trashed) and with its children back — the ones that were
  lifted to the root when it was deleted and are still there.

- **Script**: with all its versions. A script still bound (a step, a note, a template, a template step, a live hook) cannot
  be deleted; a text that embeds it (`script=N{…}`) does not hold it, and its link stays broken while the script is in the
  trash (the delete confirmation lists those texts; `GET /api/scripts/{id}/uses` returns them with `via: "text"`).
- **Hook**: stops firing while in the trash. It does not hold its script, so the script can go to the trash after it; a hook
  comes back only after its script, and erasing the script for good erases its trashed hooks.
- **A script binding** of a trashed note or step stays: the sweep of orphan bindings sees trashed rows, and only a note or a
  quest erased for good frees its script.

## How it works (so a new query needs no care)

`quest` and `note` got `deleted_at` (migration 0015), `script` and `hook` the same in 0030 (`execution/trash.go`). A **restrictive row-level-security policy** `not_trashed` hides such rows
from every ordinary query of the API role, so no `SELECT` has to know about the trash. The trash operations
(`journal/trash.go`, `knowledge/trash.go`) open the policy for one transaction with `set_config('app.trash', '1', true)`.
Dependent rows (steps, tags, attachments, secrets) are not touched until a purge.

Two things need care when a query reads a *dependent* table without joining its owner (RLS only hides the owner row):
`queststep` (graph, search labels, runner orphan jobs) and `attachment` (the Attachments tab) filter by
`quest_id IN (SELECT id FROM quest)` / the owner subselect. A new such query must do the same.

Checks and emits of a trashed quest or note do not run: the scheduler finds them through `quest` / `note`.
