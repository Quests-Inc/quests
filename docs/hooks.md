# Hooks

A hook is a workspace row (`hook`) that runs a library script ([`scripts.md`](scripts.md)) when an event happens. Owners write
hooks; members may list them. ADR-0004 has the reasoning.

## What a hook listens to

- **Events** come from one catalog (`GET /api/hook-events`, `platform/hooks/catalog.go`), in families: quest, step, note,
  variable (`values_changed`: owner, name, type, and the value for a number or a yes/no, never for a text), file
  (`attachment_added`, `attachment_revision`). The interface shows their titles and picks a whole family from its head; the
  codes are in the hints. Short aliases (`complete`, `note`…) are still accepted by the API and unfold into kinds.
- **Area** (`scope_kind`): `workspace`, `quest`, `step`, `note` or `note_tree` (a note and everything under it), with `scope_id`.
- **Limits**: `cooldown_sec` (a pause per event kind and entity; the first event runs at once, later ones wait and the newest
  runs when the pause is over; at least 30 s for `note_updated`, 5 s for `step_progress`, 2 s for `values_changed`),
  `max_per_hour` (over it the hook is switched off with `disabled_reason`), `include_private` (events of private notes are
  skipped unless asked for).

## What a hook runs

A hook script reads the event through `source.event` (typed ports by `family`, including a `ref` and `secrets` of the entity).
A graph that reads only the event runs as the workspace. Code goes to the runner the hook names (`run_on`); a script without code
runs on the server and needs none. HTTP and unix delivery use the trusted sinks `HTTP: отправить` (`sink.http`) and `sink.socket`
on the API host, through the administrator's egress policy.

**Loops.** A run descends from a chain of hooks (`origin`, `hook:3>hook:7`); what it writes to variables carries the chain in
the event, and a hook is not woken by an event its own chain made, nor by a chain of three.

## Seeing what a hook did

The hook list says when each hook last ran and how, and how many times in the last hour; a click opens its runs in Автоматика →
«Запуски» ([`scripts.md`](scripts.md#the-run-log)). «Запустить на…» in the script editor makes up an event of a chosen kind
about a chosen entity, to try a hook's script without waiting for the event.

## Deleting

A deleted hook goes to the trash and stops firing; it comes back only after its script ([`trash.md`](trash.md)).

MCP: the `hook` tool (list, get, create, update, delete, events).
