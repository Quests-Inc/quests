# Quests CLI

A client of the journal: quests and hooks through the local API.

The implementation is a **Go binary**, `go/bin/quests` (no Python or `go run` at runtime):

```bash
just build-cli                  # once (or after the CLI changes)
just quests --help
just quests list --json
go/bin/quests list              # directly
uv run quests list              # launcher → the same binary
```

Optionally put it on PATH: `cp go/bin/quests ~/.local/bin/quests`

A running API is required (`just run-server`).

| Env / flag | Default | Purpose |
|-----|---------|------------|
| `QUESTS_API` / `--api URL` | `http://127.0.0.1:8765` | the API base (`--api` overrides the env) |
| `QUESTS_API_TOKEN` | — | bearer token when the instance has accounts ([`auth.md`](auth.md)) |
| `QUESTS_WORKSPACE` | the token's workspace | which workspace a call works in (checked against membership) |

`llm-add TEXT…` sends the text to the server's assistant (`POST /api/llm/actions/preview`), shows the action plan and, after confirmation (`-y` skips the question), applies it through `/api/llm/actions/apply`. The LLM provider, key and limits are **server** settings (`GROQ_API_KEY`, `QUESTS_LLM_*`, see `docs/security.md`); the CLI does not read them. Exit code `2` means the assistant asks for a clarification (the question is on stderr).

## `--json` / `--api` / `-h`

- `-h` / `--help` — on the root and on every subcommand.
- `--json` — a machine-readable response on stdout (errors are JSON too, on stderr: `{"ok":false,"error":"…"}`).
- `--api URL` — the API base (overrides `QUESTS_API`); may come before or after the subcommand.

```bash
quests --json list
quests list --json
quests --api http://your-server:8765 list
quests show 3 --json | jq .title
```

## Quests

| Command | What it does |
|---------|------------|
| `list` / `ls` | list (`--status`, `--pinned` / `--unpinned`, `--category`, `--questline`) |
| `show` / `get ID` | details + steps (+ category / questline) |
| `add TITLE` | create (`-d`, `--pin`, `--significance`, `--step`×N, `--category`, `--questline`) |
| `llm-add TEXT…` | free text → the server's assistant → an action plan (`-y` skips the confirmation) |
| `set ID` | fields: `--title` / `-d` / `--category` / `--questline` / `--significance` (`none` clears) |
| `pin ID` / `unpin ID` | pins (`pin --off`) |
| `status ID STATUS` | `active\|expired\|frozen\|completed\|failed\|archived` |
| `complete ID` | → completed |
| `fail ID` | → failed |
| `step ID` | +1 on a step (or `--inc N` / `--set N` / `--done`; `--step-id` / `--title`) |
| `step-add QUEST TITLE` | add a step (`--total`, `--progress`, `--sort-order`, `-d`, `--quiet`) |
| `step-edit QUEST STEP` | step fields (`--title` / `-d` / `--total` / `--set` / `--sort-order`) |
| `step-rm QUEST STEP` | delete a step (not the only one) |
| `progress ID` | `{"progress": done, "total": all}` over the quest's steps: ready-made output for the `check_command` of a step that watches another quest (see “Child quests as steps” below) |
| `delete` / `rm ID` | delete |

Examples:

```bash
quests add "Gather herbs" --pin --step "Meadow" --step "Mountain"
quests add "MVP" --category work --questline "Project"
quests set 4 --category health
quests set 4 --questline none
quests step 4 --title meadow --inc 2
quests step-add 4 "New step" --total 3
quests step-edit 4 12 --title "Renamed" --total 5
quests step-rm 4 12
quests complete 4 --json
```

## Categories and questlines

| Command | What it does |
|---------|------------|
| `categories` / `cats` | the category reference (slug, label, color) |
| `questline list` / `ql ls` | list lines (`--category`) |
| `questline show ID` | details |
| `questline add TITLE` | create (`--category`, `--color`, `--icon`) |
| `questline set ID` | change (`--title` / `-d` / `--category` / `--color` / `--icon`) |
| `questline delete ID` | delete (the quests are detached, they keep their category) |

`--category` / `--questline` accept an `id`, a `slug`/`label` (for a category), a substring of the title (for a line) or `none`/`-` to clear. A member of a questline always gets the line's category.

```bash
quests categories
quests questline add "Quests project" --category work --icon flag --color '#5a8a9a'
quests ql list --category work
quests ql set 1 --category health   # syncs the category of the members
```

## Notes

Markdown pages of the knowledge base (not quests). They are tied to quests with `note=N` in the text.

| Command | What it does |
|---------|------------|
| `note list` / `notes ls` | list (`--parent ID` or `--parent none`) |
| `note show ID` | body, parent, pin |
| `note add TITLE` | create (`-d`, `--parent`, `--pin`) |
| `note set ID` | `--title` / `-d` / `--parent` (`none` moves to the root) |
| `note delete ID` | delete (children move up to the root) |

```bash
quests note add "Style guide" -d "Instructions. Data: note=2"
quests notes ls --json
```

## Hooks: workspace-wide and per quest

A hook is an **event + a script** from the library (ADR-0003). Owners manage them in the web («Хуки») or here; members can list.
Delivery is through the graph evaluator (Code on a runner, or `sink.http` / `sink.socket` on the server).

- Without `--quest` — workspace-wide: any event of the kind in this workspace.
- With `--quest ID` — only that quest's events.

Events (`--event` / `-e`, repeatable): aliases or raw kinds (`GET /api/hook-events`).

```bash
quests hook events
quests hook events --json
```

Useful aliases: `complete`, `step`, `status`, `fail`, `created`, `deleted`.

```bash
# any complete in the workspace — script 3 must read a quest (sample «Хук: команда на событие»)
quests hook add -e complete --script 3 --run-on home --name notify

# only quest #12
quests hook add -e step -e complete --script 3 --quest 12 --name daily-ping --run-on home

quests hook list
quests hook list --global
quests hook list --quest 12 --json
quests hook disable 1
quests hook enable 1
quests hook remove 1
```

The active workspace is `QUESTS_WORKSPACE` / the token's workspace (same as every other CLI command).

## Exit codes

- `0` — ok  
- `1` — an API / validation error, or the hook was not found  
- `2` — no command / bad usage  

## Child quests as steps

A parent quest's step can follow another quest. Two ways:

1. **Native (a hook, no runner).** On a saved step the field «Следит за квестом» (or `follow_quest` in MCP, `PUT /api/steps/{id}/mirror`
   with `{"quest_id": N}`) makes the server create a hook with the followed quest as its area and a small script. At every change of
   that quest (a step moved, steps added or removed, its status changed) the step's progress becomes *steps done* of *steps there
   are*, and its maximum follows; a completed parent reopens when the followed quest gains steps. A step cannot follow its own
   quest or close a circle (A follows B, B follows A, through any number of quests). The step's own check is cleared when it starts
   following; taking the link away removes the hook and the script. See [`hooks.md`](hooks.md).
2. **Polling (the old way, for a step not yet saved, or for a server without hooks).** `quest=N` in the step's description, and the
   step's `check_command` `quests progress N` in mode «watch» (`run_mode=watch`) on a runner. In the modal the hint appears by itself
   as soon as `quest=N` shows up in the step's title or description. The runner executes the check with `QUESTS_API` and a token in the
   environment, so the CLI works from inside the command.

A poll check prints a number or JSON `{"progress": N, "total": M, "description": "…"}`; every field is optional. `total` changes the step's maximum, so when the child quest gains steps the parent step's maximum follows by itself. Modes: `poll` (a step that reached its maximum is closed and no longer polled), `watch` (polling continues after closing, at most once a minute: if the child quest gained steps, the parent step reopens) and `once` (a single run). `watch` works only while the quest is `active`: a finished quest's steps are not polled, to avoid useless requests.
