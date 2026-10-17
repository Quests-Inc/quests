# ADR-0003. Scripts as graphs of typed nodes (checks, emit, hooks, the script library)

- Status: **proposed** (the design was agreed with the owner in discussion on 2026-10-17; this text fixes it before any code)
- Date: 2026-10-17
- Related: quest=440 (script library: this is its design step), quest=438 (hooks), quest=439 / ADR-0002 (the execution
  context), quest=298 (a step that follows a quest), ADR-0001, `docs/automation.md`, `docs/security.md`

## Context

Everything that runs user code is wired by hand per source (ADR-0002 moved the queue and the rules into `execution`, but each
source still has its own shape):

| What | Where it is hard-coded today |
|---|---|
| What a job is about | four nullable columns `quest_id`, `step_id`, `template_id`, `note_id` in `runner_job`; the same four in `jobSecrets` and in `CancelOrphanJobs` |
| How the answer is read | three parsers with three formats: step (`ParseCheckOutput`: an integer, the last one found in the output, or `{"progress","total","description"}`), note (`ParseNoteCheckOutput`: `0`/`1` or `{"outdated","reason"}`), emit (`parseEmitOutput`: a quest object or `null`) |
| What a result or a failure means | `apply` / `applyOnce` / `applyNote` / `execEmitRemote`: each reacts to exit code, timeout, `skipped`, `abandoned`, `canceled` in its own way |
| How a job is shaped | dedupe key, timeout, validity: literals at three call sites |

A script library (quest=440), event hooks (quest=438) and a visual editor would each add a fourth, fifth and sixth copy.
The owner's model: a script is a small graph; one block runs arbitrary code, the others are typed, fixed and checked.

## Decision

### 1. A script is a graph of typed nodes

One stored format: a directed acyclic graph, JSON, workspace-scoped, owner-edited. It is validated on save (port types,
required inputs, no cycles, the single-code-per-path rule below). There is no second "script text" representation: at run
time the only user code is the body of a `Code` node, sent to a runner as an ordinary command.

### 2. Where things run, and trust

- **The server never runs user code** (unchanged: `docs/security.md`, the `os/exec` gate of ADR-0002). A `Code` node runs on a
  runner. Every other node is Go code of ours and runs on the server.
- Each node definition carries a **trusted** flag. In v1 exactly the built-in nodes are trusted; a graph without a `Code`
  node therefore runs entirely on the server, instantly, without a runner.
- A user may build a node from other nodes (a composite). A composite is as trusted as its parts. A user-written code node
  is never trusted. Letting the host admin mark a node as trusted (and a "propose as safe" flow for the admin to review) is
  planned and cheap (the flag exists), but is **not in v1**; running reviewed user code on the server would additionally
  need a real sandbox and is a separate decision.

### 3. Tokens and the firing rule

A link carries a **token**: either a *value* or an *exit signal*.

- A node fires when every required input holds a value (and, for an "any of" group, at least one does).
- If a required input holds an exit signal, the node does not run and puts an exit signal on all its outputs: the signal
  travels down the branch. An optional input holding an exit signal means "no value".
- The `Compare` node sends the input value to exactly one of its two outputs (`yes` / `no`) and an exit signal to the other.
  Both outputs may be wired: the branch with the value runs, the other does not.
- A run is finished when every node has settled (ran, or received an exit signal, or failed). The run log keeps the state of
  each node, so "why did the step not update" is answerable.
- Failures are ports, not hidden code: a `Code` node has an output per outcome, an unconnected outcome port does nothing.

Graph shape: acyclic; a path holds at most one `Code` node (two would be a chain of jobs with state between them: not v1);
fan-out is free; fan-in only through `Any of` / `All of`.

### 4. Port types

`ref<step>`, `ref<note>`, `ref<template>` (the template and its period key), `ref<quest>`, `ref<workspace>`, `secrets`
(opaque; connects only to `Code`), `str`, `int`, `float`, `bool`, `doc` (any JSON, including `null`), `any` (accepts any
token, ignores the value: a trigger). Implicit widening only `int → float` and scalar → `doc`; everything else through an
explicit node. A constraint (for example a minimum) may sit on a port declaration.

### 5. Nodes of v1

**Sources** (server): `Step`, `Note`, `Template`, `Quest`, `Workspace`. Each gives its `ref`, its `secrets` and the fields of
the entity as separate output ports, so a `Code` node gets them without calling the API: `Step`: `title`, `description`,
`progress`, `total`, `done`; `Note`: `title`; `Template`: `title`, `period`; `Quest`: `title`, `status`, `description`;
`Workspace`: `name`. A run triggered by an event also has an optional `event: doc`. More fields later are more ports; nothing
else changes.

**Code** (runner, the only one): inputs `secrets` and parameters (environment variables); the script text is a parameter.
Outputs one per outcome: `stdout: str` (exit 0), `failed: str` (non-zero exit, with the stderr tail), `timeout: str`,
`unreachable: str` (no runner picked it up in time), `abandoned: str` (the runner vanished mid-run), `canceled: str`.

**Parse** (server): input `str`; parameters: a schema (name, type, required, constraints). Output per schema field (value, or
exit signal for an optional field that is absent or invalid) and `bad_output: str`. **Strict**: the text is read as JSON,
the whole trimmed `stdout` first (multi-line objects work), and if that is not JSON its last non-empty line (a script may log
above its result). The value must be a JSON object matching the schema, or a single scalar that fits the first field (that is
the `int → {"progress": int}` shorthand). There is no "find the last integer in the text" leniency.

**Sinks** (server):

| Node | Inputs | Effect |
|---|---|---|
| `Step: set progress` | `ref<step>`, `progress`, `total` (min 1), `description` (at least one of the three) | what `apply()` does: clamp 0..total, quest status sync, events and toasts |
| `Step: run result` | `ref<step>`, `any` trigger, `why: str?`; parameter done / not started / stopped / failed | `applyOnce`, `releaseOnce`, `stopOnce`, failing the quest |
| `Note: verdict` | `ref<note>`, `outdated: bool`, `reason: str?` | the verdict of a note check |
| `Note: check error` | `ref<note>`, `message: str` | records the error (a log line only when the text is new) |
| `Template: create quest` | `ref<template>`, `doc` (may be `null` = nothing this period) | decode the quest spec, validate, create, record the attempt |
| `Template: attempt error` | `ref<template>`, `message: str`; parameter error / timeout / bad JSON | records a failed attempt (the retry logic stays as it is) |

**Control** (server): `Compare` (type chosen, operation and operand typed in; operations per type: numbers `= ≠ < ≤ > ≥`,
strings `= ≠ contains starts-with matches` (RE2, linear time), `bool` `=`), `Any of` (the value of the first input, by input
order, that holds one), `All of` (waits for all inputs, gives an `any` trigger; the values stay on their own links).

**Next, same rules (quest=438):** sinks `HTTP request` and `Socket`; a hook is a binding with an event trigger.

### 6. Binding and trigger live outside the graph

How a script is attached to an entity (a step, a note, a template, a quest, the workspace) is the **binding**: the trigger
(an interval, once, watch, or an event), the runner(s), the timeout, the validity window, parameters. In v1 these stay where
they are (columns of a step, a note, a template). The engine, not the graph, also does the bookkeeping that belongs to the
schedule (`check_last_run_at` after any finished run, the period of an emit, de-duplication).

### 7. Secrets

- The owner kinds get a fifth: **`workspace`** (a secret row owned by the workspace itself). The chain is now
  `workspace → questline → template → quest → step`, and a note gets `workspace → note`. A secret of the workspace is
  **inherited down** by everything inside it.
- Secrets are resolved on the server when a job is handed to a runner, as today; they are never stored in a graph or in a job
  row, and only the `secrets` port of a `Code` node can receive them. The output goes through `MaskSecrets` before it
  travels on.
- Who may set secrets and commands does not change: only the owner (`kernel.Commands`). Whether a member may attach a
  script an owner published is one more rule of that same policy (quest=440).

### 8. Jobs

A job row reduces to what it is about and which script: `subject_type`, `subject_id`, `script_id` with a **version**, a small
`args` (for example the period key of an emit), plus the snapshot of the command the runner executes. The `kind`
(`poll` / `once` / `emit_pool`) and the four columns are retired by an additive migration (new columns in one release, old
ones removed in the next). **The version is fixed when the job is queued**; a later edit of the script does not change a job
that waits. Cancelling by hand stays as it is; a second action "cancel and queue again with the current version" is added
for one-shot steps and emit, where a cancel deliberately does not re-queue itself.

### 9. Existing checks

A step or a note that holds an inline `check_command` and no script is read as an implicit graph of the nodes above (the
same `Source → Code → Parse → Sink` wiring each of today's checks has), so nothing is migrated until the owner replaces the
command with a script. Parsing is strict in these implicit graphs too; **before the switch the real check commands of the
workspaces are audited** for outputs the lenient reader accepts and the strict one would not.

### 10. Execution: how a graph becomes a job

The runner stays dumb: it receives one command, an environment and limits, and returns exit code, `stdout` and `stderr`. It
knows nothing about graphs; the whole graph is evaluated on the server (a runner's claim about which branch to take could not
be trusted anyway, and an old runner would break at every new node type).

| Phase | What happens |
|---|---|
| Enqueue | The evaluator runs the nodes in front of `Code` (sources, a `Compare` used as a guard…). If `Code` gets an exit signal on an input, no job is queued and the run is settled. Otherwise the job takes the **snapshot** of the `Code` text, its environment, the timeout and the runner; the run (script version, state of every node) is stored in the database |
| Claim | The secrets are resolved and put into the environment of the job, as today. They are in no stored row |
| Complete | The evaluator resumes the stored run, puts the outcome on the ports of `Code` (`stdout` / `failed` / `timeout` / …) and goes on to the end; each node's state is recorded |

The `Code` text:
- It is plain text. A text that starts with `#!` is written to a file and executed by the runner as it already does; any
  other text goes to `sh -c`. The node has a `language` parameter (`sh` by default, `python`, `node`…) only for the editor: if
  the text has no interpreter line, the server prepends the matching one. A line in the text wins over the parameter. The
  interpreter must exist on the runner's device; a missing one is an ordinary `failed` with its stderr.
- Size: at most 64 KiB.
- The server does not wrap the user's code; the contract (the output is JSON) is checked by `Parse`, not by a wrapper.

Inputs of `Code`:
- Only through the environment, never substituted into the text (this is what keeps a parameter from becoming a command).
  A value arriving over a link is `QUESTS_IN_N<k>_<PORT>`, where `<k>` is the number of the node it comes from (a per-script
  sequence number given when the node is created, never reused; the node's uuid stays internal) and `<PORT>` the port name,
  so values from different nodes never overlap. A link can carry an optional alias (`QUESTS_IN_<ALIAS>`), unique in the node,
  for readability. The total size of the environment is capped at 32 KiB: more is an error when the graph is saved, not a
  silent cut.
- Service variables: `QUESTS_JOB_ID`, `QUESTS_API` stay; `QUESTS_SUBJECT_TYPE` and `QUESTS_SUBJECT_ID` are added.
  `QUESTS_STEP_ID` and `QUESTS_NOTE_ID` are **migrated**: they are given for one more release next to the new ones, the
  existing check commands are scanned for them beforehand, and then they are removed.

Output of `Code`: `stdout` is the only data channel. `stderr` is for logs and the text of an error. The `progress=N total=M`
line the runner reads on the fly for the progress indicator stays a separate mechanism, outside the graph.

Platforms: a non-Unix build of the runner exists, but a command still goes through `sh -c` and an interpreter line needs
`exec` of a file with a shebang, so on Windows this works only where a POSIX shell exists (Git Bash, WSL). The same holds
today; nothing in this ADR depends on it.

## What changes in ADR-0002

`execution.Kind` (`Owns`, `Finished`) becomes the **sink** interface of this ADR; the planned registry of subjects (does it
still exist, where its secrets come from) becomes the **source** nodes. Nothing built for ADR-0002 is thrown away.

## Open (to be decided after this ADR)

- The editor (comfy-ui style): the last step of quest=440.
- Composite nodes: how a subgraph is declared and versioned.
- `list<T>` as a port type, if `All of` should gather values.

## Consequences

**Pros.** One format for checks, emit, hooks and library scripts; a new source of code is nodes, not columns and parsers; the
security invariant (the server runs no user code) is a property of the node types, not of discipline; failure handling is
visible and editable instead of buried in `apply*`; the run log explains itself.
**Cons.** A real evaluator, a graph validator and a migration of `runner_job` are needed before the first user-visible gain;
the implicit graphs must reproduce today's behaviour exactly (they are the regression suite); strict parsing may reject
outputs that worked.

## Rollout order (quest=440)

1. This ADR accepted. 2. The graph model, node registry and validator (pure Go, no UI). 3. Storage and API of scripts, the
`workspace` secret owner. 4. Evaluator, the stored run and the job format; implicit graphs for today's checks; the old paths switch to it one
by one (the service variables migration `QUESTS_STEP_ID` / `QUESTS_NOTE_ID` → `QUESTS_SUBJECT_*` spans two releases). 5. The scripts tab and picking a script in a step, a note, a template. 6. Samples. 7. The graph editor. 8. Hooks
(quest=438) as one more trigger and two more sinks.

## Progress

- Step 2 (model, registry, validator): done in `go/internal/execution/graph` (pure Go: `Graph`, `Builtin()` registry of the 16 node
  types, `Validate` / `Check`, `Decode`, `EnvName`). The four checks that exist today are tests: each is a valid graph.
- Step 3 (storage, API, `workspace` secret owner): done. `script` / `script_version` (migration 0017) with versions and an
  optimistic check, `/api/scripts*` and `/api/script-nodes*`, scripts in the workspace export/import; the `workspace` owner of
  secrets (migration 0016) with `GET|PUT|DELETE /api/workspace/secrets`, inherited down to everything including notes.
- Step 4 (evaluator, stored runs, job format): done for the note check, the one-shot step and the polled step
  (`implicit:note_check`, `implicit:step_once`, `implicit:step_poll`). The polled step reads its answer strictly; the live check
  commands were looked through first and none relied on the lenient reader. Template emit stays on its older path: its pull
  model (attempts, retries, the period marker) is a separate change. Jobs keep the four entity columns for now; `subject_*`
  and the retirement of `kind` come with the removal of the older kinds (the release after this one).

- Steps 5–7 (UI): done. Scripts bind to steps and notes (`script_binding`, migration 0019); the «Скрипты» tab has the list, the
  samples (`graph/samples.go`) and the graph editor, which runs on the map's gestures, minimap and camera fit. Templates are not
  bound yet: emit is still on its older path.
