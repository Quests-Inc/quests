# ADR-0002. A context for running user code (runners, the job queue, the rules)

- Status: **accepted** (2026-10-17, the owner; the open questions are answered below)
- Date: 2026-10-17
- Related: quest=439 (this), quest=438 (hooks in the web UI), quest=440 (script library), ADR-0001, `docs/automation.md`

## Context

Automation (step checks, note freshness checks, template emit, soon event hooks and a script library) keeps growing, and
the code that makes it safe is spread over four places (measured 2026-10-17):

| Concern | Where it lives today |
|---|---|
| "Only an owner writes a command", "a command needs a runner" | `kernel/commands.go`, plus **three identical** thin wrappers `guardAuthor` / `guardExec` in `journal`, `knowledge` and `automation` stores |
| Runners, tokens, queue, leases, expiry | `automation/runner.go`, `runner_jobs.go` — inside the context that also owns templates, emit and secrets |
| What to enqueue and when | `schedule/checks.go` (steps), `notecheck.go` (notes), `emit_remote.go` / `emit_roll.go` (templates): each builds its own `JobSpec`, dedupe key, timeouts and result handling |
| Secrets of a command | `automation/secrets.go` |
| The one exception | `platform/hooks` starts processes on the API host |

The rules are written down (`docs/automation.md`) and since this ADR's gate (`archtest/exec_test.go`) the "no `os/exec` outside
an allow-list" rule is checked. Everything else is convention: a new source of user code (hooks, library scripts) would copy
a guard pair and a queue call site again.

## Decision (proposed)

A new context **`execution`** (`go/internal/execution`, own HTTP face, ADR-0001 dependency rules) that owns:

1. **Command policy** — the one place for author and runner rules (today `kernel.CommandAuthor` / `CommandExec` and their
   wrappers). Stores call it instead of keeping their own wrapper.
2. **Runners and the job queue** — registry, tokens, `EnqueueJob` / `ClaimJob` / heartbeat / `CompleteJob` / expiry /
   orphan cancelling. Moved out of `automation`.
3. **Job kinds as handlers.** A kind (`poll`, `once`, `emit_pool`, later `hook`) registers: how to build the `JobSpec`
   (command, runner, timeout, validity, dedupe key) from an entity, and what to do with the result. `schedule` decides
   *when* and asks the kind to enqueue; it no longer builds `JobSpec`s itself.
4. **Secrets of a command** — they only exist as the environment of a job, so they belong here (an amendment to ADR-0001
   "secrets belong to automation", the same way that one amended "to knowledge").

`automation` keeps templates, emit rules and attempts; `journal` and `knowledge` keep their entities and ask `execution` for
the policy and for enqueueing. Frontend: the Runners page stays; a Scripts/Hooks screen (quest=438/440) is a client of the
same context.

## Order of work (each step ships on its own and keeps the tests green)

1. This ADR accepted (or amended).
2. One policy object replaces the three wrappers (`kernel.Commands`), no behaviour change.
3. Move runners/queue/leases from `automation` to `execution` (package move + `depguard` rules).
4. Introduce the job-kind interface and port `poll`/`once`, then `emit_pool`, then the note check, deleting the three
   `EnqueueJob` call sites in `schedule`.
5. Hooks (quest=438) and library scripts (quest=440) arrive as new kinds.

The exec gate (`archtest/exec_test.go`) is already in place and stays; `platform/hooks` is the only server-side entry on its
allow-list and leaves it with step 5.

## Answers to the open questions (2026-10-17)

- `runners` and `execution` are **one** context: registration/auth of runners and job kinds share the queue, the tokens and
  the policy, and splitting them would only add an interface between things that change together.
- The policy decides **who may pick an existing script** too (quest=440): one function in `execution`, not a wrapper per store.
- Secrets move to `execution` (amends ADR-0001 once more): they exist only as the environment of a job.

## Progress

- Step 2 (one policy): done. `kernel.Commands.Check` replaces the `guardAuthor` / `guardExec` pairs; each store keeps a single
  `guardCommand` wrapper that supplies its role lookup.
- Step 3 (move): done. `go/internal/execution` (+ `execution/http`) owns runners, tokens, the queue, leases and the secrets of
  commands; `automation` keeps templates and emit. `depguard` rules for the new context are in `.golangci.yml`
  (`execution` imports `identity`, `kernel`, `platform` only; `automation` imports `execution`).
- Step 4 (kinds): done in the shape `execution.Kind` (`Name`, `Owns`, `Finished`) and `execution.Kinds`; `schedule/jobkinds.go`
  has the step, note and emit kinds. Jobs still are queued through `Store.EnqueueJob` at each kind's own site: building a
  `JobSpec` is specific to the entity, so the shared part is the outcome routing, not a spec factory.

## Consequences

**Pros.** A new source of user code is one handler, not a copy of guards and queue calls; the rules sit in one package that can be
read in a sitting; the layering is enforced by `depguard` like the other contexts.
**Cons.** A package move touching `automation`, `schedule` and the HTTP wiring; during it two layouts coexist; the job-kind
interface must fit three quite different kinds (a repeating poll, a one-shot, a template that prints a quest) — which is the
point of designing it before the third and fourth kind exist.
