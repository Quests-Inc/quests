# Quests CLI

A client of the journal: quests through the local API, plus hooks (script / webhook / unix socket).

The implementation is a **Go binary**, `go/bin/quests` (no Python or `go run` at runtime):

```bash
just build-cli                  # once (or after the CLI changes)
just quests --help
just quests list --json
go/bin/quests list              # directly
uv run quests list              # launcher → the same binary
```

Optionally put it on PATH: `cp go/bin/quests ~/.local/bin/quests`

A running API is required (`just run-server`), except for the `hook *` commands — they work on a file directly.

| Env / flag | Default | Purpose |
|-----|---------|------------|
| `QUESTS_API` / `--api URL` | `http://127.0.0.1:8765` | the API base (`--api` overrides the env) |
| `QUESTS_API_TOKEN` | — | bearer token when the instance has accounts ([`auth.md`](auth.md)) |
| `QUESTS_WORKSPACE` | the token's workspace | which workspace a call works in (checked against membership) |
| `QUESTS_HOOKS` | `data/hooks.json` | where hooks are stored |

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

## Hooks: global and per quest

**Both kinds.**

- Without `--quest` — **global**: fires on any event of the given kind.
- With `--quest ID` — only that quest's events.
- One event can fire both global and quest hooks.

Types:

| `--type` | Parameter | Behaviour |
|----------|-----------|-----------|
| `script` | `--command` | `shell=True`; the event JSON on stdin; env `QUESTS_KIND`, `QUESTS_QUEST_ID`, `QUESTS_TITLE`, `QUESTS_DETAIL`, `QUESTS_PAYLOAD` |
| `webhook` | `--url` | `POST` `application/json` |
| `socket` | `--path` | a unix stream, one JSON line + `\n` |

Events (`--event` / `-e`, repeatable): aliases or raw kinds.

```bash
quests hook events          # table of aliases → kinds
quests hook events --json
```

Useful aliases: `complete`, `step`, `status`, `fail`, `created`, `deleted` (also `on_complete` / `on_step` / `on_status_change`).

```bash
# global: any complete
quests hook add -e complete -t script \
  -c 'notify-send "Quest" "$QUESTS_TITLE"'

# only quest #12
quests hook add -e step -e complete -t script --quest 12 \
  --name daily-ping \
  -c 'echo "$QUESTS_PAYLOAD" >> /tmp/quests-12.log'

# webhook
quests hook add -e fail -t webhook --url http://127.0.0.1:9000/hook

quests hook list
quests hook list --global
quests hook list --quest 12 --json
quests hook disable <id>
quests hook enable <id>
quests hook remove <id>
```

**Workspaces.** A hook is bound to one workspace and fires only on its events: the `workspace` field in `hooks.json`
(no field = workspace 1, so old hooks keep working as before). `quests hook add … --workspace 3` is for workspace 3,
`--workspace all` is for the host admin and fires on the events of every workspace (the payload then carries the titles and descriptions of other
people's quests: use it deliberately). Hooks are configured by the host admin; they cannot be created from the interface — a `script` is a shell command on the server.

Hooks are read by the **server** on `hub.publish` (the UI, the CLI through the API, expiry, the periodic tick). The file is picked up on every dispatch — no server restart is needed after `hook add`.

## Exit codes

- `0` — ok  
- `1` — an API / validation error, or the hook was not found  
- `2` — no command / bad usage  

## Child quests as steps

A parent quest's step can watch another quest:

1. `quest=N` in the step's description (in the frontend it is a link to the quest).
2. The step's `check_command`: `quests progress N`, mode “watch” (`run_mode=watch`), with `run_on` naming a runner. In the modal the hint appears by itself as soon as `quest=N` shows up in the step's title or description.
3. The runner executes the check with `QUESTS_API` and a token in the environment, so the CLI works from inside the command.

A poll check prints a number or JSON `{"progress": N, "total": M, "description": "…"}`; every field is optional. `total` changes the step's maximum, so when the child quest gains steps the parent step's maximum follows by itself. Modes: `poll` (a step that reached its maximum is closed and no longer polled), `watch` (polling continues after closing, at most once a minute: if the child quest gained steps, the parent step reopens) and `once` (a single run). `watch` works only while the quest is `active`: a finished quest's steps are not polled, to avoid useless requests. A native link without polling is postponed: quest=298.
