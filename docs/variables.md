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

`round(value)` or `round(value, 0)` rounds to an integer; `round(value, digits)` rounds to 0–9 decimal places.
`digits` must be an integer. Numeric arguments may be expressions or variables, including nested calls.
Halves round away from zero: `round(2.5)` → 3, `round(-2.5)` → -3, `round(-1.25, 1)` → -1.3.
Zero precision returns an `int`; positive precision preserves an integer input or returns a `float`.
Calculations use binary floating point, so decimal values near a half follow their stored representation.
For example, `round(369.06000000000006, 2)` → 369.06 and `round(327.98750000000007)` → 328.

## In texts, scripts and commands

- A text shows a variable as `{{name}}` (`{{note=12.str}}` for another entity's); an embedding's formula reads it by name
  ([`embeds.md`](embeds.md)), and `?set=name` rolls a script's result into it.
- The node `Переменная: записать` (`sink.value`, params `name` and `type`, inputs `ref` of any entity and `value`) writes what a
  script computed; a variable that is a formula is not overwritten. Wire `source.note → code → parse → sink.value` for a check
  that keeps variables fresh.
- A command (a Code node) gets the effective variables of its entity as environment variables, the name in upper case; a
  secret of the same name wins, and a value that came over a link into the Code node wins over both.

## Changes

A write (the table, the API, a script node, a runner) is announced on the socket as `values_changed` with the owner and `value_type` (the envelope `type` remains `quests_changed`), and open
pages read the variables again. Hooks can wait for it (the «переменная» family, [`hooks.md`](hooks.md)).

## Where in the interface

The `{}` button in a quest's and a note's toolbar, the «Переменные» section of the quest and step forms, the questline's block,
and Settings → workspace for the workspace's own.

## Write several variables from a script

The «Переменные: записать несколько» node (`sink.values`) accepts `ref` and a JSON object on `values`. Each object key
becomes a variable name; numbers, strings and booleans determine the scalar type. Nested objects, arrays, nulls, bad names,
limits and an existing formula reject the entire write. The transaction either writes every field or writes nothing;
change events are emitted only after commit. The single-variable node keeps its existing contract.

## Дублирование заметки

«Дублировать» создаёт заметку рядом с исходной и копирует её собственные обычные переменные: типы, текущие значения, формулы и источник повторного броска. Броски заново не выполняются. Унаследованные переменные остаются унаследованными через того же родителя.

Собственные **секреты не копируются**. Секреты родителя и пространства по-прежнему доступны по обычным правилам наследования. Это правило относится к команде дублирования заметки, а не к экспорту/импорту пространства.

## Manual batch upsert

`PUT /api/{notes|quests|steps|questlines|templates}/{id}/values` (or
`PUT /api/workspace/values`) accepts `{"values":[...]}`. MCP: `set_values(owner, values)`.
Each row has `name`, explicit `type` (`int`, `float`, `str`, `bool`) and exactly one
of `value` or a nonempty `formula`. Zero, false and the empty string are literals;
null is rejected. Duplicate names are errors. Only listed own values change.

The final inherited environment is evaluated once, independent of row order, so
formulas may refer to a later row. Missing dependencies, cycles, division by zero,
type/range violations and final quotas roll back the whole transaction. Formula/literal
replacement is intentional here; automatic `sink.values` retains its stricter policy.
The owner row is locked, including for single manual writes, rolled values and
script packages. Untouched values retain their roll provenance; manually written
values clear it. No rolls or user code execute during this operation.

Response: `{"items":[...]}` in input order, with typed values and computed formula
results. Validation failures return HTTP 422 with `detail` and, for row failures,
`error:{index,name,message}` (index -1 identifies an unchanged dependent formula).
One collection change event is published after commit. Workspace writes still require
an owner; MCP cannot use this operation on private notes or read private ancestors.
Body ceiling: 256 KiB; final admission budgets come from [server limits](server-limits.md).

CLI uses the same endpoint, without temporary scripts:

```
quests --json values set note=123 values.json
cat values.json | quests values set workspace -
```

`values.json` is the typed array itself, e.g.
`[{"name":"hp","type":"int","value":20},{"name":"alive","type":"bool","formula":"hp > 0"}]`.
Use the normal `--api`/token/workspace configuration. Server validation and the
single transaction remain authoritative; a failed command does not partially write rows.
