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

## Where it runs

Like a step's check: on the server (only in a *trusted* workspace) or on a runner. The interval has the same
minimum as for steps. Environment: `QUESTS_NOTE_ID`, `QUESTS_API`, `QUESTS_API_TOKEN` and, in a non-default
workspace, `QUESTS_WORKSPACE`.

## Rights and secrets

- Only workspace owners set or change the command (same as steps).
- A note has **its own secrets** (SecretsModal, kind `note`), exported to the command as environment variables.
  They are not inherited from a quest, a step or a questline, and are sealed at rest like all secrets
  (see [security.md](security.md)).
- MCP: `set_note_check`, `check_note_now`.

## Example: a note about files in a git repo

Put `<!-- verified: <commit> -->` in the note and use `scripts/note_check_git.py`:

```bash
python3 scripts/note_check_git.py --repo ~/projects/Quests go/cmd/quests-server/admin.go
```

It is outdated when any path has a newer commit than the marker. To mark the note fresh again, edit the
marker to the commit the note now describes (`git log -1 --format=%h -- <paths>`). A missing marker or an
unknown commit is an error (exit 2).
