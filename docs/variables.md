# Variables

Named values of an entity, called **переменные** in the interface and `values` in the code, the API (`/api/…/values`) and
the names of the MCP tools (`list_values`, `set_value`, `delete_value`). Unlike secrets they are readable data: a text shows
them, a formula and a script read them.

## Owners and inheritance

A variable belongs to the workspace, a questline, a template, a quest, a step or a note. What an owner sees is its own
variables and the inherited ones, nearest first:

- a step: step > quest > template > questline > workspace (the same chain as secrets, [`secrets.md`](secrets.md));
- a note: the note > its parents > the workspace.

A name is lower-case latin, digits and `_`, up to 24 characters, starting with a letter. Types: `int`, `float`, `str`, `bool`.

## Formulas

A variable is a literal or a formula over the other variables in scope: `(str - 10) / 2`; integer `/` and `%` round down,
strings join with `+`. A formula follows the variables it names. `note=12.str` (and `quest=`, `step=`, `questline=`,
`template=`) reads a variable of another entity, in a formula and in a text.

## In texts, scripts and commands

- A text shows a variable as `{{name}}` (`{{note=12.str}}` for another entity's); an embedding's formula reads it by name
  ([`embeds.md`](embeds.md)), and `?set=name` rolls a script's result into it.
- The node `Переменная: записать` (`sink.value`, params `name` and `type`, inputs `ref` of any entity and `value`) writes what a
  script computed; a variable that is a formula is not overwritten. Wire `source.note → code → parse → sink.value` for a check
  that keeps variables fresh.
- A command (a Code node) gets the effective variables of its entity as environment variables, the name in upper case; a
  secret of the same name wins, and a value that came over a link into the Code node wins over both.

## Changes

A write (the table, the API, a script node, a runner) is announced on the socket as `values_changed` with the owner, and open
pages read the variables again. Hooks can wait for it (the «переменная» family, [`hooks.md`](hooks.md)).

## Where in the interface

The `{}` button in a quest's and a note's toolbar, the «Переменные» section of the quest and step forms, the questline's block,
and Settings → workspace for the workspace's own.
