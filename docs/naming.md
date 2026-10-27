# Naming in the interface

How the interface names things, so that a new screen reads like the old ones and the strings can later move into a catalog
for an English version (quest=496). The terms themselves are in [`glossary.md`](glossary.md).

## The rules

1. **One term per concept.** The glossary's Russian word is the only one on screen: «переменная», not «значение»; «запуск»,
   not «прогон»; «хук», not «триггер». Code may keep its own name (`values`, `script_run`): then the glossary says both.
2. **The interface is Russian.** Codes — statuses (`expired`), event kinds (`quest_completed`), node types (`source.note`),
   entity kinds (`note`), job states (`success`) — never stand alone on screen. They go into the hint (`title`) of the word
   that shows them. A shared helper turns a code into its word (`statusWord`, `eventWords`, `SOURCE_WORDS`…); an unknown code is
   shown as it is rather than hidden.
3. **What stays as typed.** Names a person gave are shown as given: runner labels, variable names, port names inside a graph,
   tags. So are proper names: HTTP, JSON, MCP, HUD, Markdown.
4. **Buttons are verbs**: «Сохранить», «Запустить на…», «Восстановить». An ellipsis means a dialog opens before anything
   happens. An icon-only button has the same verb in its hint and its `aria-label`.
5. **Tabs and sections are nouns, plural for lists**: «Скрипты», «Запуски», «Хуки»; a section of a form is what it holds
   («События», «Где слушать», «Где выполнять код»).
6. **States are short**: a participle or an adjective agreeing with the thing («просрочен» for a quest, «готово» for a run,
   «выключен» for a hook). A filter that shows everything is «Все».
7. **Empty states say what is missing and how to get it**: «Скриптов пока нет. «+» создаёт пустой.» A loading state says
   «Загрузка…» and never pretends to be empty.
8. **Errors say what happened and what to do**, in Russian, server texts included: «в скрипте есть код: выберите раннер».
9. **Time**: relative in lists («3 мин назад»), absolute in details (`formatLocal`).
10. **An entity is named with its number when that helps to find it**: «№12 · Погода». The `note=12` form belongs to markdown
    sources and to MCP, not to labels.
11. **Whole phrases, not pieces.** A sentence is one string with placeholders, never glued from fragments, and plurals go
    through one helper: the future catalog needs whole sentences to translate.

## Checking

A new screen is read once for codes on screen (rule 2). Places fixed by the review of 27.10 (quest=491): quest status in the
list, the card and on the map; the source nodes' titles («Источник: заметка»); the script list's captions (no raw subjects);
the hook form and list (event titles); the hook filter «Все»; the step chip («каждые N с» instead of «auto Ns»); runner job
states in the run log; job names of embeddings (the script's title instead of `script=N`).
