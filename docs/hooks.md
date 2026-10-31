# Hooks

A hook is a workspace row (`hook`) that runs a library script ([`scripts.md`](scripts.md)) when an event happens. Owners write
hooks; members may list them. ADR-0004 has the reasoning.

## What a hook listens to

- **Events** come from one catalog (`GET /api/hook-events`, `platform/hooks/catalog.go`), in families: quest, step, note,
  variable (`values_changed`: owner, name, type, and the value for a number or a yes/no, never for a text), file
  (`attachment_added`, `attachment_revision`), template (`template_emitted`, `template_emit_failed`, `template_period_skipped`:
  a quest was made, the emit ran out of attempts, the period passed with nothing to make; `template_id` and `period` in the
  payload, `source.event` family `template` has a `ref` to the template). The interface shows their titles and picks a whole family from its head; the
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

## A step that follows a quest

MCP: use `follow_quest({step_id:1778,quest_id:188})` to make a step mirror another quest (сделать квест шагом другого квеста / следить за квестом / зависимость).
Call it again with another `quest_id` to replace the link, or with only `step_id` to remove it.
It uses the same API as the interface, managing the hook, script, cycle checks and initial synchronization.
Do not use `script bind` or manually build a hook for this standard case. Custom graphs with different logic can still run from ordinary hooks.
`get_step` returns `follows` with `step_id`, `quest_id`, `parent_quest_id`, `hook_id`, `script_id`, or null when unlinked.
The target step's own `script_id` can be null: the mirror script belongs to the hook.

A hook made by the step field «Следит за квестом» (`hook.mirror_step_id` names the step; it goes with the step) has the followed quest
as its area, waits for `step_progress`, `step_completed`, `quest_updated`, `status_changed`, `quest_completed`, `quest_failed`, `quest_expired`,
and runs a script of three nodes: `source.quest` (the followed quest, from the area) → `source.step` with `id` of the step →
`sink.step_progress`, wired `progress`→`progress`, `total`→`total`, `ref`→`ref`. `source.quest` has the ports `progress` (steps done)
and `total` (steps; 1 for a quest without steps). Any script can do the same with `id` on a source: a source with an `id` reads that
entity of the workspace, not the one the run was started for. Limits of the link: it is made once per step (`PUT /api/steps/{id}/mirror`
replaces), refuses its own quest and circles, and a followed quest in the trash simply stops changing. The script's `id` is literal, so
a workspace restored into another one needs its links made again.

## Seeing what a hook did

The hook list says when each hook last ran and how, and how many times in the last hour; a click opens its runs in Автоматика →
«Запуски» ([`scripts.md`](scripts.md#the-run-log)). «Запустить на…» in the script editor makes up an event of a chosen kind
about a chosen entity, to try a hook's script without waiting for the event.

## Deleting

A deleted hook goes to the trash and stops firing; it comes back only after its script ([`trash.md`](trash.md)).

MCP: the `hook` tool (list, get, create, update, delete, events).
