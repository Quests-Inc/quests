# Running user code: the rules

One page for everything that makes the service run something a person wrote: a step's check, a note's freshness check,
a template's `emit_pool_command`, and (planned, quest=438) event hooks. The details of each live in their own pages
(see the end); this page is the contract they all follow.

## The rules

1. **The server executes no user commands.** `quests-server` never starts a process from text a user or an owner
   wrote. Its own fixed processes (`pg_dump` for backups) are not user code and take no input from the database.
2. **Code runs on a runner only.** A runner is `quests-runner` on a device the workspace owner registered (the
   server's own host included, as one more device). It runs with the rights of the user it was started as; that
   machine's owner is the one who decides what it may do.
3. **Only an owner writes a command.** A member can read and edit everything around a command (title, progress,
   description) but not set or change the command itself. A command stored without a runner is never run: the step
   stays idle and the note or template reports "choose a runner". Automation as an actor is refused too.
4. **A command names its runner.** `run_on` is one label, or an ordered list `a,b` of fallbacks (the first one online
   wins; queued jobs are re-routed when it goes away). No runner, no command.
5. **A runner belongs to one workspace.** A registration token is one-time and expires; it is swapped for a permanent
   token, which is revocable. A runner takes jobs by its own label only.
6. **Everything goes through the job queue.** The scheduler (or an event) *enqueues* a job; a runner *claims* it
   under a lease, heartbeats, and *completes* it. The server never calls into a runner.
7. **Secrets are the command's environment, nothing else.** Owned by the workspace, a questline, template, quest, step or note and
   inherited downwards (`docs/secrets.md`): workspace > questline > template > quest > step, and workspace > note; only owners set them; names that would bend how a process starts
   (`PATH`, `LD_*`, `BASH_ENV`, `QUESTS_*`…) are refused and never exported; values are sealed at rest (`platform/secretbox`, `docs/secrets.md`).
8. **A failure is recorded, never hidden.** A note check that errors stores the error text and keeps the previous
   verdict; an emit error is logged as an attempt (`emit_attempts`); a `once` step check that exits non-zero or times
   out fails the quest on purpose, with the exit code and output as a comment; a job nobody finishes expires.

## Life of a job

```
owner writes command + run_on          (rules 3, 4: kernel.Commands.Check)
        │
scheduler / event ── EnqueueJob ──►  runner_job (state queued, dedupe_key, valid_until, timeout)
                                          │  one open job per dedupe_key: a slow runner is never piled up
runner ── ClaimJob (lease) ──► run ──► JobHeartbeat (progress, log tail) ──► CompleteJob (result)
                                          │  lease not renewed → lost; waited past valid_until → skipped;
                                          │  lost and no result before valid_until → abandoned (ExpireJobs)
                                          │  quest/step/template deleted → CancelOrphanJobs
server applies the result in the owning context (step progress, note verdict, emitted quest)
```

Job kinds: `poll` (repeating check), `once` (single check), `emit_pool` (template prints a quest). Queued jobs of a
kind have their own validity window and timeout.

## Where it lives in the code

| Concern | Place |
|---|---|
| Who may write a command, "a command needs a runner" | `internal/kernel/commands.go` (`kernel.Commands.Check`: author, then runner); each store (`journal`, `knowledge`, `automation`) has one `guardCommand` wrapper that supplies its role lookup |
| Runners, tokens, queue, leases, expiry | the `execution` context (ADR-0002): `internal/execution/runner.go`, `runner_jobs.go`; HTTP `execution/http/runners.go` |
| Runner label rules | `internal/kernel/runlabel.go` |
| What to enqueue and when | `internal/schedule`: `checks.go` (steps), `notecheck.go` (notes), `emit_remote.go` / `emit_roll.go` (templates). Each source is an `execution.Kind` (`schedule/jobkinds.go`): it owns the jobs it queued and receives their outcome; a new source (hooks, library scripts) is one more `Kind` |
| Secrets for a command | `internal/execution/secrets.go` |
| The executor | `cmd/quests-runner` (`agent.go`, `exec*.go`) |

## Scripts (ADR-0003)

A script is a graph of typed nodes stored per workspace (`script`, one row per script; `script_version`, one row per saved
graph: a job that waits for its runner runs the version it was queued with). The model, the node catalog and the validator
are in `internal/execution/graph`; the store is `internal/execution/scripts.go`; the API (`internal/execution/http/scripts.go`):

| Route | |
|---|---|
| `GET /api/scripts`, `GET /api/scripts/{id}`, `GET /api/scripts/{id}/versions/{n}` | read (any member) |
| `POST /api/scripts`, `PUT /api/scripts/{id}` (`version` = the one the editor read, else 409), `DELETE /api/scripts/{id}` | write (owners only) |
| `POST /api/scripts/validate` | check a graph without saving it |
| `GET /api/script-nodes`, `POST /api/script-nodes/ports` | the node catalog and the ports a node gets from its parameters (for the editor) |

**In the UI** the scripts live in the «Скрипты» tab: a list, samples to start from (`GET /api/script-samples`) and an editor
on the map's own field (its gestures and minimap; `frontend/src/pages/scripts`). Nodes are dragged, linked from an output to an
input, their parameters are edited on the right, the validator answers while you draw, and a save that only moves nodes
rewrites the current version instead of adding one. A step or a note picks a script in its auto-check form
(`PUT|DELETE /api/steps|notes|templates/{id}/script`; a template step carries `script_id` in its own column and the quest made from it is bound to the same script, and a step created with a quest or on its own may name one in `script_id`); the script then takes the place of the command, and the device, mode and interval
stay the step's own.

**MCP:** one tool `script` with actions `list` / `get` / `create` (graph or `sample`) / `update` / `delete` / `validate` /
`nodes` / `samples` / `bind` / `unbind` (`subject` = step|note|template). A new step may take `script_id` on
`add_step` / `create_quest`; attaching a script to an existing step or note is `script` bind. Graph format:
[`adr/0003-script-graphs.md`](adr/0003-script-graphs.md).

A script has a **purpose**, chosen by hand (`purpose`: `step`, `note`, `template`, `hook` or empty = «Общие»). It sorts the
«Скрипты» tab and narrows the pickers: a step offers the scripts for steps, plus unsorted ones whose sources read steps. The
`script` tool takes `purpose` on create and update.

A graph is validated on save (types of ports, required inputs, cycles); a bad one is a 422 listing
the problems per node. Steps, notes and template emit run through the evaluator (see Runs below).


### Custom nodes

In the editor: right-click the field → «Добавить ноду» → group → node (own nodes are the last group). «Новая нода из кода…» (also
in the palette) makes a node of one `Code` node: its inputs become variables (the name in capitals), its fields fixed variables
the script edits on the node. Several picked nodes can be baked into one («Запечь выделенное…»): links leaving the selection
become inputs and outputs, and any scalar parameter of the picked nodes (a source's entity number, a code variable) can be kept
as an editable field. A node holding code runs on a runner. «Новый скрипт» makes an empty script named «Новый скрипт» (an empty one can be saved, not bound or run); «Из ноды» in a form makes a script of one ready node; the former samples are the starting nodes («Поставить стартовые ноды», `custom_starters`), and right click → «Добавить граф» puts a sample or another script into the open one with its nodes unfolded. A baked node is edited in its form (name, field labels and values) or unfolded into the script and baked again over it. Details and limits: ADR-0003 §6a; `script` tool actions `custom_*`.

### Runs (ADR-0003, stage 4)

A graph runs in `execution/graph` (tokens, firing rules, strict `Parse`); `schedule/scriptrun.go` starts a run, queues a job for
each ready `Code` node and resumes the run when the job comes back (`scriptKind` in `schedule/jobkinds.go`). Between the two
the run lives in `script_run` as a snapshot, so a server restart loses nothing; the job points at it (`runner_job.run_id`,
`run_node`) and carries the values that came over links in `runner_job.env` (secrets are not stored: they are resolved when a
runner claims the job). Finished runs are kept three days as a log.

What a plain `check_command` stands for is an *implicit graph* built in `scriptrun.go`; the old paths are switched to them one by one:

| Check | Graph | State |
|---|---|---|
| note freshness | `implicit:note_check` | through the evaluator |
| one-shot step (`run_mode=once`) | `implicit:step_once` | through the evaluator |
| polled step | `implicit:step_poll` | through the evaluator. The answer must be JSON (a number or an object, as the whole output or its last line); the older reader also took the last integer found in free text. The live steps were looked through before the switch: none relied on it |
| template emit (`emit_pool_command`) | `implicit:template_emit` | through the evaluator. The tick still owns the period. Nothing printed, `{}` or `null` is no quest this period. A device that never came, and a cancel, close the period with no attempt and no retry; any other failure is an attempt that is retried. A job queued before this (no run) is still pulled the old way until it finishes |
| template emit by a script (`script_binding`, subject `template`) | the bound graph (`TemplateEmit` is the sample) | the same tick and the same sinks. The run's key is `emit:<template>\|<period>\|<script>.<version>` |

Jobs queued before runs existed have no `run_id` and are served by the older kinds until they finish.

### Computing inside a text: `script=N{…}`

A markdown text (a note, a step, the map's cards) may hold `script=12{5+3к8}`: a live link that, on click, calls script 12 with the
text in braces and shows `5+3к8 = 19`; a next click throws again, nothing is stored. `script=12{8+pb+str}?auto` is the same
embedding with a parameter: the value is computed when the text is shown, and again when that text is exported (Markdown or PDF).
`?hide` hides the formula. With `?auto` the reader sees only the value (`script=12{8+pb+str}?auto&hide`); without it the link shows `?` until a click, then only the value. Either order works (`?hide&auto`).
A click throws again. Up to 16 `?auto` embeddings per text are computed; the rest stay click-only. The parameter is explicit —
a formula is never guessed to be safe to run. The script is `Args → … → Result` made of
server nodes only (`expr`, `random`, `regex`, `compare`, `parse`…); a graph with a `Code` node is refused. The text goes to the first
parameter of `Args` (`POST /api/scripts/{id}/eval {"args": {"_": "…"}}`, or named args; MCP `script` action `eval`).
Dice are not a feature of the server: the example script is `Args → Expr → Result`, and the `Expr` node has a table of operators
(`к`: `sum(a, rand(1, b))` for «N dice with b sides», `к` as a prefix for one die; the same for `d`), so `5+3к8` is parsed by precedence and computed in one node.
Limits and the reasoning: ADR-0003 §6b.

## Known exceptions (to be removed or kept deliberately)

- **Event hooks** are workspace rows (`hook` + a library script). Matching events enqueue a `script_run` / runner job;
  Code runs on the runner. HTTP and unix delivery use trusted sinks `sink.http` and `sink.socket` on the API host
  (SSRF host checks apply). Owners write hooks; members may list them.
- **`dry_run_emit_pool` in the MCP server** runs the command on the machine where the MCP server runs (the user's
  own, over stdio) to preview a template; it does not touch the API host.
- **The HUD** (`internal/overlay`) runs local window-manager calls on the workstation it is installed on; it is not
  part of the server.

## Related pages

[`security.md`](security.md) (trust boundaries) · [`secrets.md`](secrets.md) · [`emit.md`](emit.md) ·
[`note-check.md`](note-check.md) · [`architecture.md`](architecture.md) · ADR-0001 (shared kernel).

### Menus and picking in the editor

- Right click on the field: «Добавить ноду» (group → node), «Добавить граф» (samples, scripts), «Новая нода из кода…», and with two or more picked, «Запечь выделенное…».
- Right click on a card: delete; for a custom node also edit and unfold; «Запечь в ноду…». On one of several picked cards: only delete (all picked).
- Right click on a script in the list: rename, purpose (second menu), «Запечь в кастомную ноду…» (a copy of the whole script), delete.
- Ctrl / Cmd picks without the selection mode, in the script editor and on the map: ctrl+click a card, ctrl+drag a frame.
