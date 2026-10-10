# Quests

Менеджер задач в духе игрового журнала: веб-UI, API и Wayland-оверлей (HUD + тосты) для niri / CachyOS.

Стек: **Go (API + CLI) · PostgreSQL · Svelte (Vite) · Python (MCP / Telegram / STT) · GTK4 + gtk4-layer-shell**

## Требования

- Go ≥ 1.26 (сборка API/CLI)
- Docker (локальный PostgreSQL: `just dev-db`)
- Python ≥ 3.12, [uv](https://docs.astral.sh/uv/) (миграции, MCP, Telegram)
- Bun (фронт)
- Для оверлея: `gtk4`, `gtk4-layer-shell`, `python-gobject` (Wayland)

```bash
# Arch / CachyOS (оверлей)
sudo pacman -S gtk4 gtk4-layer-shell python-gobject
```

## Развёртывание

```bash
just bootstrap
```

Нужен [`just`](https://just.systems). Список всех рецептов (сборка, запуск, `just check` = линтеры + тесты) — просто `just`.

Подтянет Python-зависимости (`uv sync --group dev --all-extras`), соберёт Go CLI/API, `bun install` во `frontend/`, создаст `data/` и накатит миграции.

## Запуск

| Что | Команда | URL / заметка |
|-----|---------|----------------|
| API + SPA | `just run-server` | http://127.0.0.1:8765 (нужен собранный `frontend/dist`) |
| Сборка API | `just build-server` | → `go/bin/quests-server` |
| Фронт (HMR) | `just run-frontend` | http://127.0.0.1:5173 (нужен запущенный API) |
| Сборка SPA | `just build-frontend` | в `frontend/dist` |
| Схема БД | `just schema` | сервер сам мигрирует схему при старте; `quests-server schema` показывает ревизию (нужен `QUESTS_DATABASE_URL`; локально — `just dev-db`) |
| CLI | `just quests …` / `go/bin/quests` | см. [`docs/cli.md`](docs/cli.md) |
| Оверлей | `just run-overlay` | или `python -m overlay` |
| Telegram | `just run-telegram` | HTTP-клиент к API |
| STT (голос → текст) | `just run-stt` | сайдкар для бота, http://127.0.0.1:8766, нужен для голосовых сообщений |
| Шаблоны | UI «Шаблоны» / `/api/templates` | daily/weekly → инстансы-квесты |

Оверлей (IPC):

```bash
python -m overlay toggle    # passthrough ↔ interactive
python -m overlay monitor   # следующий монитор
python -m overlay status
```

Тема / позиция HUD сохраняются в `data/overlay.json` (`style_pack`, `monitor_connector`, `margins`). Переключение стиля — в interactive-режиме (dropdown). Env `QUESTS_STYLE_PACK` перекрывает конфиг на старте.

```bash
QUESTS_STYLE_PACK=cyberpunk just run-overlay
```

Layer-shell namespaces (для `niri msg layers` / `layer-rule`): `quests-overlay` (HUD), `quests-major`, `quests-minor`.

```kdl
// ~/.config/niri/config.kdl — пример
layer-rule {
    match namespace="^quests-"
    // block-out-from "screencast"
}
```

## Доступ

Локальный инстанс на `127.0.0.1` работает без логина, как раньше. Как только в
БД появляется аккаунт — API закрывается целиком, а SPA просит войти:

```bash
quests-server useradd <username>   # пароль спрашивается интерактивно
quests-server token add overlay    # bearer-токен для HUD / бота / CLI / MCP
```

Наружу инстанс без аккаунтов не поднимется — сервер откажется биндиться не на
loopback. Подробности: [`docs/auth.md`](docs/auth.md).

База — PostgreSQL (`QUESTS_DATABASE_URL`); локально `just dev-db` поднимает её в Docker, схему накатывает сам сервер при старте (см. [`docs/postgres.md`](docs/postgres.md)).

User systemd units: [`deploy/systemd/`](deploy/systemd/).  
Деплой на сервер после клона: [`deploy/SERVER.md`](deploy/SERVER.md).  
Docker (api / bot / frontend): [`deploy/docker/`](deploy/docker/).  
HUD на удалённый API (токен, unit, niri): [`docs/hud-workstation.md`](docs/hud-workstation.md).  
Релизы (SemVer) и деплой на прод: [`deploy/README.md`](deploy/README.md) — прод обновляется только явным GitHub Release.

> **Не запускайте голый `uv sync`.** Он синхронизирует окружение точно и удаляет необязательные пакеты (`mcp`, `aiogram`, `faster-whisper`):
> `quests-mcp`, бот и STT тогда падают с `No module named …`. Всегда `uv sync --group dev --all-extras` (так делает `just bootstrap`).
