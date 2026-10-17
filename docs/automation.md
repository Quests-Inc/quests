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

A graph is validated on save (types of ports, required inputs, cycles, one `Code` node per path); a bad one is a 422 listing
the problems per node. Nothing runs a script yet: the evaluator and the attaching of scripts to steps, notes and templates
are the next stages of quest=440.


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
| template emit (`emit_pool_command`) | not yet | older path (pull model with attempts and retries) |

Jobs queued before runs existed have no `run_id` and are served by the older kinds until they finish.

## Known exceptions (to be removed or kept deliberately)

- **Event hooks** (`internal/platform/hooks`, `data/hooks.json`) still execute `script` hooks on the server's own host
  and `webhook` / `socket` from the server process. They are the host admin's, not a workspace owner's. Moving them
  under these rules is quest=438.
- **`dry_run_emit_pool` in the MCP server** runs the command on the machine where the MCP server runs (the user's
  own, over stdio) to preview a template; it does not touch the API host.
- **The HUD** (`internal/overlay`) runs local window-manager calls on the workstation it is installed on; it is not
  part of the server.

## Related pages

[`security.md`](security.md) (trust boundaries) · [`secrets.md`](secrets.md) · [`emit.md`](emit.md) ·
[`note-check.md`](note-check.md) · [`architecture.md`](architecture.md) · ADR-0001 (shared kernel).
