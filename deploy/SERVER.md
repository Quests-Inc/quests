# Деплой Quests на сервер (после `git clone`)

Два пути: **Docker** (проще на VPS) или **systemd + uv** (как на рабочей станции).

Оверлей (HUD) на сервере **не** нужен — только API + SPA (+ опционально Telegram-бот).  
HUD остаётся на рабочей станции с Wayland и ходит на сервер через `QUESTS_API`.

---

## Вариант A — Docker (рекомендуется на сервере)

Прод ставится и обновляется через релизы и doco-cd: см. [`README.md`](README.md); сам стек (compose, настройки, секреты)
живёт в репозитории [Quests-Inc/quests-deploy](https://github.com/Quests-Inc/quests-deploy) — для ручного запуска без
doco-cd клонируйте его и следуйте его README.

Дальше — firewall на `8765`/`8080`, на ПК HUD: `QUESTS_API=http://SERVER_IP:8765`.

---

## Вариант B — systemd (после клона без Docker)

Ниже путь клонирования: `~/Quests`. Если другой — правь `WorkingDirectory` / `ExecStart` в unit-файлах.

Локальная разработка в `~/Documents/projects/Quests`:

```bash
ln -sfn ~/Documents/projects/Quests ~/Quests
```

---

## 0. Зависимости на сервере

```bash
# Go (API + CLI)
# Arch: sudo pacman -S go
# или https://go.dev/dl/

# uv: https://docs.astral.sh/uv/  (миграции, Telegram, MCP)
curl -LsSf https://astral.sh/uv/install.sh | sh

# Node.js / npm (сборка SPA)
# Arch: sudo pacman -S nodejs npm
# Debian/Ubuntu: sudo apt install nodejs npm
```

Для бота нужен исходящий доступ к Telegram **через HTTP-прокси**
(`QUESTS_TG_PROXY`, по умолчанию `http://127.0.0.1:12334`). Подними любой
локальный прокси на этом порту (или укажи свой URL в `.env`) до старта бота.

---

## 1. Клон и bootstrap

```bash
cd ~
git clone <repo-url> Quests
cd Quests

just bootstrap
just build-frontend
```

`just bootstrap`: `uv sync`, Go CLI/API build, `npm install`, `data/`, миграции DB.  
`just build-frontend`: `frontend/dist` — отдаёт Go API на `:8765`.

---

## 2. Конфиг `.env`

```bash
cp .env.example .env
$EDITOR .env
```

Минимум для удалённого API:

```bash
# Слушать не только localhost
QUESTS_HOST=0.0.0.0
QUESTS_PORT=8765

# Telegram (если бот на этом же хосте)
QUESTS_TG_TOKEN=…
QUESTS_TG_USER_IDS=…          # через запятую
# QUESTS_TG_PROXY=http://127.0.0.1:12334

# API base для бота (локально можно не трогать)
# QUESTS_API=http://127.0.0.1:8765
```

CORS нужен только если Vite/SPA открывают с другого origin:

```bash
# QUESTS_CORS_ORIGINS=https://quests.example.com,http://LAN_IP:5173
```

Открой firewall / security group на TCP `8765` (или поставь nginx/caddy перед API).

---

## 3. Проверка руками

```bash
just run-server
# в другом терминале:
curl -sS http://127.0.0.1:8765/api/health
# с другой машины:
curl -sS http://SERVER_IP:8765/api/health
```

Бот (после заполнения TG-переменных; сначала STT-сайдкар, если нужны голосовые):

```bash
just run-stt &
just run-telegram
```

В веб-UI чипы **API / HUD / Bot**: Bot зелёный после heartbeat (~несколько секунд).

Остановка: `Ctrl+C`, дальше — systemd.

---

## 4. systemd (user units)

### 4.1. Пути в unit-файлах

В репо units рассчитаны на `%h/Quests` (т.е. `~/Quests`).  
Если клон в другом месте — поправь `WorkingDirectory` и `ExecStart` во всех `*.service`, либо сделай симлинк:

```bash
ln -sfn "$PWD" ~/Quests
```

Для локальной копии в `~/Documents/projects/Quests` — либо симлинк, либо правь пути обратно.

### 4.2. Установка units

```bash
cd ~/Quests

mkdir -p ~/.config/systemd/user
ln -sf "$PWD/deploy/systemd/user/"*.service ~/.config/systemd/user/

# user-сервисы без активной сессии (сервер / SSH):
sudo loginctl enable-linger "$USER"

systemctl --user daemon-reload
systemctl --user enable --now quests-server.service
systemctl --user enable --now quests-telegram.service   # если бот здесь
systemctl --user enable --now quests-stt.service        # если нужны голосовые в боте

systemctl --user status quests-server.service
systemctl --user status quests-telegram.service
systemctl --user status quests-stt.service
```

Логи:

```bash
journalctl --user -u quests-server.service -f
journalctl --user -u quests-telegram.service -f
journalctl --user -u quests-stt.service -f
```

`quests-overlay.service` на сервере **не** включай (нужен Wayland).

---

## 5. Рабочая станция (HUD)

Оверлей — на ПК с Wayland/niri, не на сервере. Логин/пароль SPA **не**
подходят: нужен bearer `QUESTS_API_TOKEN` (на Docker-сервере:
`docker exec quests-api quests-server token add overlay-pc`).

Полный чеклист (env, systemd, niri, отладка): **[`../docs/hud-workstation.md`](../docs/hud-workstation.md)**.

Кратко:

```bash
# на ПК после токена в ~/.config/quests/overlay.env:
ln -sfn "$PWD" ~/Quests   # если клон не в ~/Quests
# unit + EnvironmentFile — см. docs/hud-workstation.md
systemctl --user import-environment WAYLAND_DISPLAY XDG_RUNTIME_DIR DISPLAY NIRI_SOCKET
systemctl --user enable --now quests-overlay.service
```

Чип **HUD** в SPA / `GET /api/health` → `components.overlay.status=ok`.

---

## 6. Обновление на сервере

```bash
cd ~/Quests
git pull
just bootstrap          # deps + migrate
just build-frontend
systemctl --user restart quests-server.service
systemctl --user restart quests-telegram.service   # если включён
```

---

## 7. Бэкапы БД / вложений и восстановление

Сервер сам раз в `QUESTS_BACKUP_INTERVAL_HOURS` часов (по умолчанию 24) снимает
консистентный дамп PostgreSQL (`pg_dump --format=custom`, файл `quests-….dump`) в `<data>/backups/`,
собирает рядом `quests-…-attachments.tar.zst` по путям из **этого** снимка
(GET с WebDAV), и:

1. если настроен WebDAV — заливает `.dump` в `/backups/` на том же WebDAV;
2. если заданы `QUESTS_BACKUP_REMOTE_HOST` + `QUESTS_BACKUP_REMOTE_DIR` — пушит
   пару (`.dump` + `.tar.zst`) по SFTP на ПК (ключ `QUESTS_BACKUP_REMOTE_KEY`,
   порт `QUESTS_BACKUP_REMOTE_PORT`, по умолчанию 22).

Ротация — `QUESTS_BACKUP_KEEP` последних пар (по умолчанию 14), список в
таблице `backuplog`. Если ПК выключен, локальные файлы остаются в
`data/backups/`, `remote_uploaded=0`, следующий тик сначала дожимает pending.
Статус — чип **backup** в `GET /api/health` (`webdav=` / `remote=`).

### Восстановление БД

```bash
# список на WebDAV
curl -s -u "$QUESTS_WEBDAV_USER:$QUESTS_WEBDAV_PASS" \
  -X PROPFIND -H "Depth: 1" "$QUESTS_WEBDAV_URL/backups/" | grep -o '<D:href>[^<]*'

# скачать дамп
curl -s -u "$QUESTS_WEBDAV_USER:$QUESTS_WEBDAV_PASS" \
  -o quests-restore.dump "$QUESTS_WEBDAV_URL/backups/quests-20260927-030000-123456789.dump"

# восстановить в чистую БД (см. docs/postgres.md → Backups)
docker compose stop api
docker compose exec -T postgres createdb -U quests quests_restored
docker compose exec -T postgres pg_restore -U quests --no-owner -d quests_restored < quests-restore.dump
# затем переключить api на quests_restored или перенести данные; не поверх живой БД
docker compose start api
```

Локальная копия последнего бэкапа лежит и на самом сервере в `data/backups/`.

### Восстановление вложений с ПК

Пара на ПК: `{REMOTE_DIR}/quests-…​.dump` + `{REMOTE_DIR}/quests-…​-attachments.tar.zst`.
После подмены БД распаковать архив в корень WebDAV с сохранением путей
(`attachments/quest-N/…`):

```bash
# с ПК или со staging сервера
tar -I zstd -tf quests-…-attachments.tar.zst   # проверить пути
tar -I zstd -xf quests-…-attachments.tar.zst -C /path/to/webdav/root
```

Либо PUT каждый файл обратно через WebDAV API / `curl --upload-file`.
Согласованность: брать `.db` и `.tar.zst` с **одним** timestamp.

---

## 8. Секреты для пользовательских скриптов шаблонов (`emit_pool_command`)

Скрипт шаблона (см. `emit_pool_command`) читает креды из окружения процесса.
Общие на все скрипты секреты — в корневом `.env` сервера (уже загружены
`config.LoadDotenv`). Секрет, специфичный для одного шаблона, — через API,
не через `.env`:

```bash
# записать (перезаписывает, если ключ уже был)
curl -X PUT http://127.0.0.1:8765/api/templates/5/secrets/MAIL_PASSWORD \
  -H "Content-Type: application/json" -d '{"value":"..."}'

# посмотреть, какие ключи заведены (значения не возвращаются никогда —
# ни этим эндпоинтом, ни GET /api/templates/{id}, ни MCP list_templates/get_template)
curl http://127.0.0.1:8765/api/templates/5/secrets

# убрать
curl -X DELETE http://127.0.0.1:8765/api/templates/5/secrets/MAIL_PASSWORD
```

Хранится как есть, без шифрования — сервер сам является границей доверия
(скрипты и так крутятся на нём же, как и с корневым `.env`). Значение
попадает в окружение дочернего процесса только в момент запуска скрипта.

---

## Порядок (кратко)

| # | Где | Действие |
|---|-----|----------|
| 1 | сервер | `git clone` → `bootstrap` → `build-frontend` |
| 2 | сервер | `.env` (`QUESTS_HOST=0.0.0.0`, TG-токены) |
| 3 | сервер | firewall :8765 |
| 4 | сервер | `enable --now quests-server` (+ `quests-telegram`, `quests-stt`) |
| 5 | ПК | `QUESTS_API` / `api_base` → оверлей |
| 6 | браузер | `http://SERVER_IP:8765` — проверить API/HUD/Bot |

Проверка здоровья: `GET http://SERVER_IP:8765/api/health`.

## Раннер проверок на другом устройстве

Автопроверки шагов (`check_command`), команды шаблонов (`emit_pool_command`) и проверки заметок выполняются только на **раннерах**: сервер сам ничего не запускает. Раннер ставится на любое устройство, включая сам сервер (отдельным пользователем): он сам ходит на сервер исходящим HTTPS (long-poll), белый IP и открытые порты не нужны.

```bash
# на сервере: завести раннер и получить одноразовый токен (24 ч)
quests-server runner add home          # метка: a-z, 0-9, _ и -, до 32 символов
# то же из UI: вкладка «Раннеры» → «Добавить»

# на устройстве: собрать и поставить
just build-runner                     # → go/bin/quests-runner
install -Dm755 go/bin/quests-runner ~/.local/bin/quests-runner
quests-runner register --server https://quests.example --token <токен>
ln -sf "$PWD/deploy/systemd/user/quests-runner.service" ~/.config/systemd/user/
systemctl --user enable --now quests-runner.service
```

Метку указывают в поле «устройство» автопроверки шага (`run_on`) или у команды шаблона (`emit_run_on`); без метки команда не сохранится и не запустится. Команда исполняется от пользователя раннера (`sh -c` в его домашней папке; скрипт с `#!` — своим интерпретатором), секреты шага/шаблона приходят переменными окружения.

- **Офлайн:** опрос пропускает такт; разовая проверка ждёт устройство до суток и не проваливает квест; команда шаблона ждёт в окне периода. Устройство, пропавшее посреди выполнения разовой проверки, даёт понятный провал (повторять небезопасно).
- **Прогресс долгих проверок:** команда может печатать строку `progress=N total=M`, раннер передаёт её и хвост лога на сервер (вкладка «Раннеры»).
- **Отзыв:** `quests-server runner rm home` или корзина во вкладке; задания в очереди отменяются.
- **Ограничение:** задания несут секреты и произвольные команды, поэтому раннер доверяет серверу; ставьте его под отдельным пользователем с нужными правами.
