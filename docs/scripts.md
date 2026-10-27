# Scripts

A script is a graph of typed nodes (ADR-0003): a **source** reads an entity, **Code** runs on a runner, **parse** checks
the answer, **logic** and **control** nodes compute and branch on the server, a **sink** writes the result. The same graph
serves a step's or a note's check, a template's emit, a hook ([`hooks.md`](hooks.md)) and an embedding in a text
([`embeds.md`](embeds.md)). The rules every run follows are in [`automation.md`](automation.md).

## The library

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


## Custom nodes

In the editor: right-click the field → «Добавить ноду» → group → node (own nodes are the last group). «Новая нода из кода…» (also
in the palette) makes a node of one `Code` node: its inputs become variables (the name in capitals), its fields fixed variables
the script edits on the node. Several picked nodes can be baked into one («Запечь выделенное…»): links leaving the selection
become inputs and outputs, and any scalar parameter of the picked nodes (a source's entity number, a code variable) can be kept
as an editable field. A node holding code runs on a runner. «Новый скрипт» makes an empty script named «Новый скрипт» (an empty one can be saved, not bound or run); «Из ноды» in a form makes a script of one ready node; the former samples are the starting nodes («Поставить стартовые ноды», `custom_starters`), and right click → «Добавить граф» puts a sample or another script into the open one with its nodes unfolded. A baked node is edited in its form (name, field labels and values) or unfolded into the script and baked again over it. Details and limits: ADR-0003 §6a; `script` tool actions `custom_*`.

## Runs

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

## The run log

Every run lands in `script_run` with what started it: `source_kind` = `step`, `note`, `template` (a bound script or an
implicit check), `hook`, `embed` (a text's `script=N{…}` with a Code node) or `test`, and `source_id` (migration 0031). Server-only
embeddings computed on show or click (`?auto`, a click without code) are not stored: they are many and change nothing.

| Route | |
|---|---|
| `GET /api/runs?script_id=&source_kind=&source_id=&subject_type=&subject_id=&state=&before=&limit=` | a page of the log, newest first, with the titles of the script, the source and the subject |
| `GET /api/runs/{id}` | one run: every node of its graph with its state, failure and the values on its outputs (`exits` = outputs that carried nothing), and the runner jobs of its Code nodes |
| `GET /api/run-stats?kind=hook\|step\|note\|template` | the newest run of each source of a kind and how many started in the last hour (the hook list carries it as `runs`) |
| `POST /api/scripts/{id}/try` | a test run (owner): `subject {type,id}`, `event` (a made-up event of that kind for `source.event`), `args` (a script with `Args`), `graph` (the unsaved graph, stored on the run as `inline`), `on` (the runner for code), `live` |

A test is **dry** unless `live`: its sinks run and write nothing (`dryHost`); the snapshot still shows what came into them.
Code goes to the runner for real either way. Finished runs are kept three days, and past that the newest 50 of each source stay
(`RunsKeptPerSource`), so a hook that fires once a week still shows what it did.

UI: Автоматика → «Запуски» (filters by state and source; a jump from another tab narrows it to one script or one source); the
script editor has «Запустить на…» and «Запуски этого скрипта»; a hook in the list says when it last ran and how; «Раннеры →
Недавние» folds the jobs of one source into one line. MCP: `script` actions `runs`, `run`, `try`.

## Outgoing HTTP

A graph may read the outside on the server, with no runner: `HTTP: запрос` (`http.get`: the owner fixes the host, the inputs
fill only the path and the query values) and `JSON: достать` (`json.get`). Whether a workspace may do it is the administrator's
decision (`QUESTS_EGRESS_*`, `quests-server egress enable <id>`; ADR-0004, `docs/security.md`).

The same request can be sent by a runner instead: `HTTP: запрос (раннер)` (`http.runner`, the second node of the «Код на
раннере» group next to Code). It goes out from the runner's machine, so the server's policy and the workspace grant do not
apply and it reaches a local network; what limits it is set in the node: `allow` (hosts the request and its redirects may
touch, empty means no limit), `timeout_sec` (20, up to 300), `max_bytes` (256 KiB, up to 400 KiB), `no_redirect`, `headers`,
`scheme` (https or http). The job's command is the marker line `#!quests-http` and a JSON request; a runner without this
node's support would run it as a shell command and fail. Outputs and `failed` are as in `http.get`.

In a query value `{path}` is the text that arrived on the `path` input and `{path.0}`, `{path.1}` are its parts between commas,
so `script=17{55.75,37.62}` can fill `lat={path.0}&lon={path.1}`; a piece used in the query is not added to the path.
`json.get` of type `str` takes a number or a flag as text.

## The editor

The palette groups are Sources, Code on a runner, Parse, Control (compare, any/all of), Logic (expression, random, regex,
JSON, HTTP), Sinks, Custom; a group folds into its head, and so does a node in the editor (kept in the browser, not in the
script). With nothing selected, the right panel lists where the script is used (steps, notes, templates, template steps,
hooks, and the texts that embed it), each a link. «Запустить на…» and «Запуски этого скрипта» are next to «Сохранить».

### Menus and picking

- Right click on the field: «Добавить ноду» (group → node), «Добавить граф» (samples, scripts), «Новая нода из кода…», and with two or more picked, «Запечь выделенное…».
- Right click on a card: delete; for a custom node also edit and unfold; «Запечь в ноду…». On one of several picked cards: only delete (all picked).
- Right click on a script in the list: rename, purpose (second menu), «Запечь в кастомную ноду…» (a copy of the whole script), delete.
- Ctrl / Cmd picks without the selection mode, in the script editor and on the map: ctrl+click a card, ctrl+drag a frame.
