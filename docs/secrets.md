# Secrets

Values (passwords, tokens) for the commands a runner executes: a template's `emit_pool_command` and a step's `check_command` (the server runs no commands itself, see [`security.md`](security.md)). A secret is an environment variable of the command.

## Levels and inheritance

A secret belongs to one of these owners: the **workspace** itself, a **questline**, a **template**, a **quest**, a **step** or a **note**. A command sees the secrets of its own owner and of everyone above it, from the particular to the general:

```
step  >  quest  >  template (quest.template_id)  >  questline  >  workspace
```

- A secret with the same name on a more particular owner overrides the general one.
- A template's `emit_pool_command` sees the secrets of the template and of its questline.
- A step's `check_command` sees the secrets of the step, of its quest, of the template that created the quest and of the quest's questline.
- Changing a password is done in one place (on the most general owner); there is no need to copy it across quests.

When an owner is deleted, its secrets are deleted with it. The exception: when a **template** is deleted, its secrets are copied onto its still-open quests that have steps with an auto-check (otherwise those checks would lose their password, since `quest.template_id` is cleared).

## UI

The «Секреты» (Secrets) menu in the header, and the «Секреты» item in the context menu (right click) of a quest, a step, a questline and a template (in the template list): it opens on that entity. In the «Чьи секреты» (Whose secrets) field you can type `@title` (it inserts `quest=12`, `template=5`, …) or paste a reference; only entities that can own secrets are offered (questline, template, quest, step). The list shows own names and inherited ones with their source.

## API and MCP

`GET|PUT|DELETE /api/{questlines|templates|quests|steps|notes}/{id}/secrets[/{key}]` and, for the workspace you work in, `GET|PUT|DELETE /api/workspace/secrets[/{key}]`. The list returns only names: `{"keys": [...own...], "inherited": [{"key", "from", "id"}]}`. Values cannot be read through the API or MCP.

MCP: `list_secrets(owner)`, `set_secret(owner, key, value)`, `delete_secret(owner, key)`, where `owner` is `questline=N`, `template=N`, `quest=N`, `step=N`, `note=N` or `workspace` (the active one). The old `*_template_secret` tools work as before.

A secret's name is a valid environment variable name (`[A-Za-z_][A-Za-z0-9_]*`). Names that would bend how a process starts (`PATH`, `LD_*`, `BASH_ENV`, `QUESTS_*`…) are refused.

## Masking

Everything a command prints (stdout, stderr, traces, the emit attempts' log) passes through a replacement of secret values with `***` (values shorter than 4 characters are not masked). This protects against an accidental leak, not against intent: base64 or splitting the value defeats masking.

## At rest and risk

Values are sealed in the database with the server key (AES-256-GCM, see [`security.md`](security.md)): a dump or a backup alone does not reveal them. A runner receives them in the environment of the command, on the owner's own device.

Whoever can create or change a step with a command in a questline can print its secrets (only owners of a workspace can write commands). Keep a secret on the narrowest owner that needs it. A secret of the workspace is seen by every command inside it (notes included), so it is for things the whole workspace may use, not for a single project.
