# Emitting quests from a template

A template creates a quest on a schedule (`freq`/`weekdays`, mode `fixed` or `surprise`). By default the quest is built from the template's fields. If `emit_pool_command` is set, **the command prints the quest**. It runs on a runner (`emit_run_on`, one device or an ordered list of fallbacks; see [`note-check.md`](note-check.md)), never on the server.

## Output format

One JSON object shaped like the body of `POST /api/quests`. What it sets overrides the template's fields; the rest comes from the template:

```json
{
  "title": "Phone bill",
  "description": "markdown, quest=N and note=N as usual",
  "significance": "uncommon",
  "pinned": false,
  "automated": true,
  "questline": "Routine",
  "category": "home",
  "tags": ["infra", "api"],
  "deadline_at": "2026-10-03T12:00:00Z",
  "duration_seconds": 3600,
  "steps": [
    {"title": "Pay"},
    {"title": "Wait for the e-mail", "progress_total": 3, "check_command": "quests progress 300", "run_on": "main-pc", "run_mode": "watch"}
  ]
}
```

- `questline`, `category` and `tags` can be given by id or by name (an exact match wins over a substring; an ambiguity is an error). A questline brings its own category.
- The deadline is absolute only: `deadline_at` (+ `duration_seconds`); without them the template's rules apply.
- No `steps`: the template's steps are used.
- `null`, `{}` or empty output mean “no quest for this period” (not an error).
- Fields outside the format are ignored and their names go into the attempt's log (`list_emit_attempts`), so the format can grow without breaking anything.
- The old format (a JSON array) is no longer accepted: it is a parse error. Weight, the number of items to pick and repeat protection are gone.

## Repeats

There is no protection, deliberately: while a letter is unread, a quest about it will keep coming back. A hash of the title will not do (different letters share a subject), and repeats happen to motivate closing tasks in time. If that proves unacceptable in practice, the question will be reopened.

## Limits

By default `max_steps` is 30, `max_title` 200, `max_description` 20,000 and `max_command` 2000. A template can override any of them in the JSON field `emit_limits` (in the template's UI, the «Контент» (Content) section): `{"max_steps": 50}`. A typo in a key or a non-positive number is an error on save.

## Errors and retries

A command that fails, a timeout (20 s), invalid JSON or an invalid quest (limits, an unknown questline/tag, a bad significance) is a failed attempt. The pauses between attempts are 1/5/15 minutes, 4 attempts in all; after that the next emit creates a quest with the status `failed`, whose description holds the trace of the last attempt (secret values masked). Every attempt is logged: `GET /api/templates/{id}/emit-attempts`, MCP `get_template(include=["emit_attempts"])`.

A runner that never shows up is not a failed attempt: the job stays `skipped` as a marker for the period, nothing is re-queued and the template is not marked as failing.

## Where a quest came from

The field `quest.source` shows how a quest was created: `template:<id>` for emitted ones, otherwise the client from the `X-Quests-Source` header (`web`, `cli`, `mcp`, `telegram`, `assistant`; `api` by default).

## Secrets

The command and the steps of an emitted quest see the secrets of the template and of its questline, see [`secrets.md`](secrets.md).
