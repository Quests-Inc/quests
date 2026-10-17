# Quests — systemd user units

Полный порядок деплоя на сервер после клона: **[`../SERVER.md`](../SERVER.md)**.

## Установка units

Репо ожидается в `~/Quests` (пути в `*.service`: `%h/Quests`).  
Иначе поправь `WorkingDirectory` / `ExecStart` или сделай `ln -sfn "$PWD" ~/Quests`.

```bash
mkdir -p ~/.config/systemd/user
ln -sf "$PWD/deploy/systemd/user/"*.service ~/.config/systemd/user/
sudo loginctl enable-linger "$USER"   # на сервере без GUI-сессии
systemctl --user daemon-reload
```

## Сервисы

| Unit | Назначение | Где |
|------|------------|-----|
| `quests-server.service` | API + SPA `:8765` | сервер / локально |
| `quests-telegram.service` | Telegram-бот | сервер или любая машина с API+proxy |
| `quests-overlay.service` | Wayland HUD | **только** ПК с графикой |
| `quests-frontend-dev.service` | Vite HMR | опционально, разработка |

```bash
systemctl --user enable --now quests-server.service
systemctl --user enable --now quests-telegram.service
# на рабочей станции с niri:
# systemctl --user import-environment WAYLAND_DISPLAY XDG_RUNTIME_DIR DISPLAY NIRI_SOCKET
# systemctl --user enable --now quests-overlay.service
```

## Переменные (сервер)

В `quests-server.service` или в `~/Quests/.env`:

```bash
QUESTS_HOST=0.0.0.0
QUESTS_PORT=8765
# QUESTS_CORS_ORIGINS=…
```

Бот читает `.env` сам (`QUESTS_TG_TOKEN`, `QUESTS_TG_USER_IDS`, `QUESTS_TG_PROXY`, `QUESTS_API`).

## Удалённый HUD

Нужны `QUESTS_API` / `QUESTS_WEB_URL` и при включённом auth — `QUESTS_API_TOKEN`
(логин SPA недостаточно). Удобно через drop-in:

```bash
# ~/.config/systemd/user/quests-overlay.service.d/override.conf
[Service]
EnvironmentFile=-%h/.config/quests/overlay.env
```

Пошагово (токен на Docker/systemd, niri-бинды, чеклист ошибок):
[`../../docs/hud-workstation.md`](../../docs/hud-workstation.md).

## Health

`GET /api/health` — API + heartbeats overlay/telegram. В UI: чипы API / HUD / Bot.
