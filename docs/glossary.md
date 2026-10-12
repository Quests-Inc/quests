# Glossary

The Russian term (as in the interface) — the name in code — the meaning. The context is where it lives per [ADR-0001](adr/0001-architecture.md).

| Term (UI) | In code | Context | Meaning |
|---|---|---|---|
| Журнал (Journal) | journal | journal | The main screen: the list of quests with a detail view |
| Квест (Quest) | quest | journal | A task with a status, significance, deadline and steps |
| Шаг (Step) | step | journal | A part of a quest with progress; may have a check command |
| Квестлайн (Questline) | questline | journal | A group of related quests with a shared colour and icon |
| Раздел (Category) | category | journal | A category of quests |
| Тег (Tag) | tag | journal | A cross-cutting label of a quest or template |
| История (History) | changelog | journal | The change log of entities |
| Заметка (Note) | note | knowledge | A document in a tree; flags README, PRIVATE, CATEGORY, OUTDATED |
| Вложение (Attachment) | attachment | knowledge | A file of a quest, questline or note; versions, ClamAV scan, stored on WebDAV |
| Ссылка (Reference) | ref | knowledge | A mention of an entity in text: `quest=N`, `note=N`, `attachment=N`… |
| Секрет (Secret) | secret | knowledge | An environment value for a command; inherited step > quest > template > questline |
| Шаблон (Template) | template | automation | The rule of a periodic quest (schedule, mode, pool) |
| Эмит (Emit) | emit | automation | Creating a quest from a template; with `emit_pool_command` a command prints the quest |
| Проверка (Check) | check | automation | A step's or note's command that determines progress or freshness |
| Раннер (Runner) | runner | automation | A process on the owner's device that executes commands; the server runs none |
| Задание (Job) | job | automation | A unit of runner work |
| Пространство (Workspace) | workspace | identity | An isolated set of data; an account has several |
| Роль (Role) | role | identity | Owner or member of a workspace |
| Токен (Token) | token | identity | A bearer token for clients without a session (CLI, HUD, bot, MCP) |
| Карта (Map) | graph | cross-cutting | The map of relations between entities |
| Черновик карты (Map draft) | graph draft | cross-cutting | Edits on the map not applied until saved |
| Хук (Hook) | hook | platform | An administrator's command on an event (`data/hooks.json`) |
| HUD / оверлей (Overlay) | overlay | outside the core | A Wayland panel with pinned quests and toasts |
