# Trash

Deleting a quest or a note (the web UI, `DELETE /api/quests/{id}`, `DELETE /api/notes/{id}`, the CLI, the bot, MCP
`delete_note`) **moves it to the trash**; nothing is erased. Items stay for 30 days, then the scheduler erases them
(`httpapi.PurgeExpiredTrash`, hourly).

| | |
|---|---|
| Web | Settings → Корзина: list, restore, erase one, empty |
| API | `GET /api/trash/quests`, `GET /api/trash/notes`; `POST /api/trash/{quests,notes}/{id}/restore`; `DELETE /api/trash/{quests,notes}/{id}` (erase for good, attachments too); `DELETE /api/trash/{quests,notes}` (empty) |
| MCP | `trash` — `action: list` or `restore` (`kind`, `id`) |

## What comes back

- **Quest**: with its steps, tags, secrets, attachments and history, as it was. A questline deleted meanwhile is dropped from
  it; a template period that was emitted again meanwhile detaches the restored quest from the template.
- **Note**: under its parent (at the root if the parent is gone or trashed) and with its children back — the ones that were
  lifted to the root when it was deleted and are still there.

## How it works (so a new query needs no care)

`quest` and `note` got `deleted_at` (migration 0015). A **restrictive row-level-security policy** `not_trashed` hides such rows
from every ordinary query of the API role, so no `SELECT` has to know about the trash. The trash operations
(`journal/trash.go`, `knowledge/trash.go`) open the policy for one transaction with `set_config('app.trash', '1', true)`.
Dependent rows (steps, tags, attachments, secrets) are not touched until a purge.

Two things need care when a query reads a *dependent* table without joining its owner (RLS only hides the owner row):
`queststep` (graph, search labels, runner orphan jobs) and `attachment` (the Attachments tab) filter by
`quest_id IN (SELECT id FROM quest)` / the owner subselect. A new such query must do the same.

Checks and emits of a trashed quest or note do not run: the scheduler finds them through `quest` / `note`.
