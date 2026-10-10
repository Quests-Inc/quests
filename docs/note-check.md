# Freshness check of a note (quest=370)

A note can carry a command like a step's check. It decides whether the note is still true; a note that
is not gets an **OUTDATED** badge, can be filtered, and sinks in search ranking.

## Contract

The command prints either `0` / `1` (`1` = outdated) or JSON:

```json
{"outdated": 1, "reason": "2 newer commits in go/cmd/quests-server/admin.go since c993cd3"}
```

`outdated: null` or other output is an error. **Errors never change the verdict** — only the error text
is stored, the previous status stays.

## Hand-set flag

OUTDATED is a flag like README / PRIVATE / CATEGORY: a red clock-with-exclamation icon in the note's header, in the
«Флаги» submenu of the context menu and next to the note in the tree and in search results.
`PUT /api/notes/{id}/outdated {"outdated": true, "reason": "…"}` (reason defaults to «Отмечена вручную»). It is the
same state a check writes: a note that has a check gets the check's verdict at its next run.

## The command field

In the note's check and in a step's check the command is a multi-line, resizable field. It may be several
lines (`sh -c`) or a script with its own interpreter: a command that starts with `#!` is written to a private
temp file and executed as is (on the runner), like a template's pool command.

## Where it runs

Like a step's check: on a runner (the command needs a runner label). The interval has the same
minimum as for steps. Environment: `QUESTS_NOTE_ID`, `QUESTS_API`, `QUESTS_API_TOKEN` and, in a non-default
workspace, `QUESTS_WORKSPACE`.

## Rights and secrets

- Only workspace owners set or change the command (same as steps).
- A note has **its own secrets** (SecretsModal, kind `note`), exported to the command as environment variables.
  They are not inherited from a quest, a step or a questline, and are sealed at rest like all secrets
  (see [security.md](security.md)).
- MCP: `set_note_check`, `check_note_now`, `set_note_outdated` (hand-set flag); `list_notes(flags=…)` and `search_notes(flags=…)` filter by `outdated`, `-outdated`, …

## Example: a note about files in a public GitHub repo

The reference commit lives **in the command itself**, not in the note: you re-pin it by hand when you have
rewritten the note, so nothing is trusted blindly. The command needs only `curl` and `jq` (no checkout, no saved
script, so it also runs on a runner without the repository). One compare call tells which files changed since the pin:

```sh
REF=9aac35e
PATHS="docs/search.md go/internal/store/search.go"
curl -sf "https://api.github.com/repos/OWNER/REPO/compare/$REF...main" | jq -c --arg p "$PATHS" --arg r "$REF" \
  '($p|split(" ")) as $w | [.files[].filename | select(. as $f | $w | index($f))] as $c
   | if ($c|length) > 0 then {outdated:1, reason:("после " + $r + " изменено: " + ($c|join(", ")))} else {outdated:0} end'
```

An unknown `REF` or an API failure prints nothing, which is an error (the verdict stays). The unauthenticated GitHub
API allows 60 requests an hour per address — fine for an hourly check; for a private repository or a busier
schedule put a token in the note's secrets and add `-H "Authorization: Bearer $GITHUB_TOKEN"`. The compare API lists
at most 300 files, so a huge range may hide a change: re-pin before it gets that far.
