# Рабочая станция: HUD на удалённый API

Оверлей (GTK4 + layer-shell) живёт **на ПК с Wayland/niri**. API может быть
локальным или на сервере (systemd / Docker / tailnet). Ниже — типичный кейс:
API уже крутится где-то в сети, HUD поднимаем локально через user systemd.

## Почему «просто логин» не хватает

| Клиент | Как ходит в API |
|--------|-----------------|
| Браузер (SPA) | сессия по куке после `POST /api/auth/login` |
| HUD / CLI / бот / MCP | `Authorization: Bearer …` из `QUESTS_API_TOKEN` |

Логин/пароль аккаунта **не** подставляются в оверлей. Токен минтится на
машине с БД и показывается **один раз** (в БД только хеш). Подробности:
[`auth.md`](auth.md).

Проверка, что auth включён:

```bash
curl -sS http://SERVER:8765/api/auth/state
# {"auth_required":true,...}  →  нужен токен для HUD
```

## 1. Bearer-токен

**Из интерфейса (проще всего):** Настройки → Подключения → создать токен «HUD» — см. [connecting-clients.md](connecting-clients.md). Ниже — то же с хоста сервера.

**Docker (compose `quests-api`):**

```bash
ssh USER@SERVER
docker exec quests-api quests-server token ls
docker exec quests-api quests-server token add overlay-pc
# сохранить строку QUESTS_API_TOKEN=… — второй раз не отдаст
```

**systemd / бинарь на хосте:**

```bash
quests-server token add overlay-pc
```

Старый токен `overlay` в `token ls` без секрета бесполезен — либо новый
`token add …`, либо `token rm <id>` и заново.

## 2. Env на ПК

```bash
mkdir -p ~/.config/quests
umask 077
cat > ~/.config/quests/overlay.env <<'EOF'
QUESTS_API=http://SERVER:8765
QUESTS_WEB_URL=http://SERVER:8765
QUESTS_API_TOKEN=<секрет>
EOF
chmod 600 ~/.config/quests/overlay.env
```

Опционально `api_base` в `data/overlay.json` (env важнее файла). Токен в json
не класть.

Проверка до запуска HUD:

```bash
set -a && source ~/.config/quests/overlay.env && set +a
curl -sS -H "Authorization: Bearer $QUESTS_API_TOKEN" "$QUESTS_API/api/health"
```

## 3. Репо и systemd unit

Юниты ждут дерево в `~/Quests`:

```bash
# если клон лежит иначе:
ln -sfn /path/to/quests ~/Quests

mkdir -p ~/.config/systemd/user/quests-overlay.service.d
ln -sfn ~/Quests/deploy/systemd/user/quests-overlay.service \
  ~/.config/systemd/user/quests-overlay.service

cat > ~/.config/systemd/user/quests-overlay.service.d/override.conf <<'EOF'
[Service]
EnvironmentFile=-%h/.config/quests/overlay.env
EOF
```

Deps оверлея (Arch/CachyOS): `gtk4`, `gtk4-layer-shell`, `python-gobject`.

## 4. Старт

В графической сессии (иначе нет `WAYLAND_DISPLAY`):

```bash
systemctl --user import-environment WAYLAND_DISPLAY XDG_RUNTIME_DIR DISPLAY NIRI_SOCKET
systemctl --user daemon-reload
systemctl --user enable --now quests-overlay.service
systemctl --user status quests-overlay.service
```

Ожидание: `components.overlay.status == "ok"` в `/api/health` (чип HUD в SPA).

Разовый смоук без systemd: `just run-overlay` с теми же env —
для отладки. Для постоянной работы — только unit.

## 5. Бинды niri

IPC: `python3 -m overlay toggle|monitor|status` (сокет
`$XDG_RUNTIME_DIR/quests-overlay.sock`). Запускать из корня репо:

```kdl
// ~/.config/niri/config.kdl
Mod+Space hotkey-overlay-title="Quests HUD: toggle" {
    spawn-sh "cd ~/Quests && python3 -m overlay toggle"
}
Mod+Shift+Space hotkey-overlay-title="Quests HUD: next monitor" {
    spawn-sh "cd ~/Quests && python3 -m overlay monitor"
}

layer-rule {
    match namespace="^quests-"
}
```

## Чеклист «почему не встаёт»

| Симптом | Что проверить |
|---------|----------------|
| `401` / пустой HUD | нет / протух `QUESTS_API_TOKEN`; auth required |
| unit inactive, GTK errors | не сделан `import-environment` Wayland |
| `Overlay socket not found` | daemon не запущен (`quests-overlay.service`) |
| unit падает на `~/Quests/...` | нет симлинка / неверный `WorkingDirectory` |
| heartbeat offline | токен/API ок в curl, но env не в unit (нет drop-in) |
| бинд niri молчит | `cd` в несуществующий путь в `spawn-sh` |

## Связанное

- Деплой API на сервер: [`../deploy/SERVER.md`](../deploy/SERVER.md)
- User units: [`../deploy/systemd/README.md`](../deploy/systemd/README.md)
- Auth / токены: [`auth.md`](auth.md)
