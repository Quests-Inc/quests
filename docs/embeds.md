# Scripts inside a text: `script=N{…}`

A markdown text (a note, a quest, a step, a questline, a template, the map's cards) may embed a script. The script is
`Args → … → Result` ([`scripts.md`](scripts.md)); the text in braces goes to the first parameter of `Args`
(`POST /api/scripts/{id}/eval {"args": {"_": "…"}}`, or named args; MCP `script` action `eval`). Limits and the reasoning:
ADR-0003 §6b.

## Forms of a link

| Link | What happens |
|---|---|
| `script=12{5+3к8}` | a click calls the script and shows `5+3к8 = 19`; the next click throws again; nothing is stored |
| `script=12{8+pb+str}?auto` | computed when the text is shown, and again when it is exported (Markdown or PDF) |
| `…?hide` | hides the formula: with `?auto` the reader sees only the value; without it the link shows `?` until a click. Either order works (`?hide&auto`) |
| `script=12{2к8+2}?set=hp` | a click writes the result into the variable `hp` of the text's owner ([`variables.md`](variables.md)) |
| `script=12{55.75,37.62}?on=stand` | the script has a Code node: a click starts a run on a runner |

The parameter is explicit — a formula is never guessed to be safe to run. Up to 16 `?auto` embeddings per text are computed;
the rest stay click-only. Server-only embeddings computed on show or click are not stored as runs.

## Export

An exported text is what the view shows: `{{name}}` is its value, a formula has the variables it names in place of the names
(`2к4+2`), and `?auto` adds the result (`2к4+2 = 7`); a click-only embedding is just the formula, since a file has no click.

## `?set`: rolling into a variable

`POST …/values/hp/roll`: the server computes and stores the number, the client never sends it. The variable must already exist
and must not be a formula; the result must fit its type. The variable remembers the script and the expression, so the table can
roll it again, and any formula over `hp` follows. `?set` never runs on show, so it overrides `?auto`. A hand-typed value
forgets the roll.

## `?on`: code on a runner

A script with a Code node runs on a runner. The runner is the workspace variable `runner` (Settings → workspace → «Раннер по
умолчанию»; only the owner writes it; one label or a chain `a,b` where the first one online gets the job), else the
workspace's only runner; `?on=stand` or `?on=a,b` in the link overrides it, but only with labels of registered runners of the
workspace (anything else is a 422 that lists them). The job goes out (a queued job wakes the runners' long polls, so pick-up
takes milliseconds), and the page asks `GET /api/script-runs/{id}` until `Result` has its text (`POST /api/scripts/{id}/eval`
with `on` answers 202 and the run). It never runs on show (`?on` beats `?auto`), a workspace has at most 6 such runs waiting,
and a run with no runner online fails after two minutes. `?on` needs the code to be written by an owner like any command;
whoever may open the text may click it. Such runs are in the run log with the source «вставка».

## Dice

Dice are not a feature of the server: the example script is `Args → Expr → Result`, and the `Expr` node has a table of
operators (`к`: `sum(a, rand(1, b))` for «N dice with b sides», `к` as a prefix for one die; the same for `d`), so `5+3к8` is
parsed by precedence and computed in one node.

## Deleting a script

A text does not hold its script: a script embedded only in texts goes to the trash, and the confirmation names the texts whose
links break until it is restored ([`trash.md`](trash.md)). `GET /api/scripts/{id}/uses` lists them with `via: "text"`.
