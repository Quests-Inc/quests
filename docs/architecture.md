# Архитектура

Из чего состоит Quests, на чём каждая часть написана и как они общаются.

## Компоненты

| Компонент | Язык | Где | Что делает |
|---|---|---|---|
| **API-сервер** `quests-server` | Go | `go/cmd/quests-server`, `go/internal/*` | HTTP API, раздача SPA, планировщик (шаблоны, проверки заметок, бэкапы), события; сам накатывает схему БД при старте |
| **CLI** `quests` | Go | `go/cmd/quests` | Клиент API в терминале (`docs/cli.md`) |
| **Раннер** `quests-runner` | Go | `go/cmd/quests-runner` | Исполняет команды проверок и эмита на устройстве владельца; сервер команд не исполняет (`docs/security.md`) |
| **Веб-интерфейс** | Svelte 5 + TypeScript | `frontend/` | SPA; типы ответов API генерируются из Go (`just gen-types`) |
| **MCP-сервер** `quests-mcp` | Python | `src/quests/mcp_server.py` | Инструменты для ИИ-агентов поверх HTTP API |
| **Telegram-бот** | Python | `src/quests/telegram/` | Уведомления, быстрое добавление, голос через STT |
| **STT** `quests-stt` | Python | `src/quests/stt_server.py` | Распознавание речи (faster-whisper) отдельным контейнером |
| **Оверлей** | Python + GTK4 | `overlay/` | Wayland-HUD и тосты на рабочей станции |
| **Android HUD** | Kotlin | `android/` | Foreground-сервис с ongoing-уведомлением поверх того же REST API |

Всё, что не API, общается с сервером **только по HTTP** (`/api/*`, токен или сессия): прямого доступа к БД у них нет.

## Данные

PostgreSQL 17 — единственная БД (`docs/postgres.md`). Изоляция пространств — row-level security, роль `quests_app`
(`docs/security.md`). Схема — встроенные SQL-миграции `go/internal/db/sql/pg`; версия лежит в таблице `alembic_version`
(имя осталось от Alembic, которым когда-то создавалась боевая БД; самого Alembic в репозитории нет).

## Образы и выкатка

Три образа собираются из одного `deploy/docker/Dockerfile`: `quests-api` (Go-бинарь + SPA, без интерпретаторов),
`quests-bot` и `quests-stt` (Python). Релиз `vX.Y.Z` → образы в GHCR → PR в репозиторий `quests-deploy` → doco-cd на
сервере подтягивает изменения (`deploy/README.md`).

## Что осталось на Python и почему

Бот, MCP, STT и оверлей — отдельные процессы со своим жизненным циклом и тяжёлыми зависимостями (aiogram, whisper, GTK).
Переписывать их ради единого языка не планируется: границей служит HTTP API, и они не влияют ни на схему данных, ни на
образ API. Python в API-образе нет совсем.
