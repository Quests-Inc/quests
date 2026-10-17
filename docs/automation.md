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
7. **Secrets are the command's environment, nothing else.** Owned by questline, template, quest, step or note and
   inherited downwards (`docs/secrets.md`); only owners set them; names that would bend how a process starts
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
| Runners, tokens, queue, leases, expiry | `internal/automation/runner.go`, `runner_jobs.go`; HTTP `automation/http/runners.go` |
| Runner label rules | `internal/kernel/runlabel.go` |
| What to enqueue and when | `internal/schedule`: `checks.go` (steps), `notecheck.go` (notes), `emit_remote.go` / `emit_roll.go` (templates) |
| Secrets for a command | `internal/automation/secrets.go` |
| The executor | `cmd/quests-runner` (`agent.go`, `exec*.go`) |

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
