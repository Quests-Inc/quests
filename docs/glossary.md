# Glossary

The Russian term (as in the interface) — the name in code — the meaning. The context is where it lives per [ADR-0001](adr/0001-architecture.md).

| Term (UI) | In code | Context | Meaning |
|---|---|---|---|
| Журнал (Journal) | journal | journal | The main screen: the list of quests with a detail view |
| Квест (Quest) | quest | journal | A task with a status, significance, deadline and steps |
| Шаг (Step) | step | journal | A part of a quest with progress; may have a check (a command or a script) |
| Квестлайн (Questline) | questline | journal | A group of related quests with a shared colour and icon |
| Раздел (Category) | category | journal | A category of quests |
| Тег (Tag) | tag | journal | A cross-cutting label of a quest or template |
| История (History) | changelog | journal | The change log of entities |
| Заметка (Note) | note | knowledge | A document in a tree; flags README, PRIVATE, CATEGORY, OUTDATED |
| Вложение (Attachment) | attachment | knowledge | A file of a quest, questline or note; versions, ClamAV scan, stored on WebDAV |
| Ссылка (Reference) | ref | knowledge | A mention of an entity in text: `quest=N`, `note=N`, `attachment=N`… |
| Секрет (Secret) | secret | knowledge | An environment value for a command; inherited step > quest > template > questline > workspace |
| Шаблон (Template) | template | automation | The rule of a periodic quest (schedule, mode, pool) |
| Эмит (Emit) | emit | automation | Creating a quest from a template; with `emit_pool_command` a command prints the quest |
| Проверка (Check) | check | automation | A step's or note's command that determines progress or freshness |
| Раннер (Runner) | runner | execution | A process on the owner's device that executes commands; the server runs none |
| Задание (Job) | job | execution | A unit of runner work |
| Пространство (Workspace) | workspace | identity | An isolated set of data; an account has several |
| Роль (Role) | role | identity | Owner or member of a workspace |
| Токен (Token) | token | identity | A bearer token for clients without a session (CLI, HUD, bot, MCP) |
| Карта (Map) | graph | cross-cutting | The map of relations between entities |
| Черновик карты (Map draft) | graph draft | cross-cutting | Edits on the map not applied until saved |
| Хук (Hook) | hook | execution | An event (quest, step, note, variable, file) in an area → a library script; limits against storms ([hooks.md](hooks.md)) |
| Скрипт (Script) | script | execution | A graph of typed nodes, versioned; runs as a check, an emit, a hook or an embedding ([scripts.md](scripts.md)) |
| Нода (Node) | node | execution | One step of a script: source, code, parse, control, logic, sink |
| Своя нода (Custom node) | custom node | execution | A node baked from other nodes or from one Code node, with editable fields |
| Запуск (Run) | script run | execution | One pass of a script over an entity: what started it, what each node did ([scripts.md](scripts.md#the-run-log)) |
| Проверка из редактора (Test run) | try | execution | «Запустить на…»: a run over a chosen entity or event; its sinks write nothing unless asked |
| Вставка (Embedding) | embed | execution | `script=N{…}` in a text: computed on click or on show ([embeds.md](embeds.md)) |
| Переменная (Variable) | value, named value | execution | A readable named value of an entity, inherited, maybe a formula ([variables.md](variables.md)) |
| Исходящий запрос (Egress) | egress | execution | What a script sends out from the server; allowed by the administrator ([ADR-0004](adr/0004-egress-and-hooks.md)) |
| Корзина (Trash) | trash | cross-cutting | Deleted quests, notes, scripts and hooks for 30 days ([trash.md](trash.md)) |
| HUD / оверлей (Overlay) | overlay | outside the core | A Wayland panel with pinned quests and toasts |
