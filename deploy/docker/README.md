# Quests — Docker (сервер: api + bot + frontend)

HUD в Docker нет — на рабочей станции: `just run-overlay` и
`quests-overlay.service`.

## Сервисы

| Сервис | Порт | Описание |
|--------|------|----------|
| `api` | `8765` | Go API + собранный SPA + миграции при старте (`quests-server migrate`) |
| `bot` | host net | Telegram-бот → `127.0.0.1:8765`, прокси `127.0.0.1:12334` |
| `frontend` | `8080` | nginx → проксирует всё на `api` |
| `clamav` | internal `3310` | clamd sidecar; сигнатуры в volume `clamav-sigs` |

```bash
cd /path/to/Quests
cp -n .env.example .env
# QUESTS_TG_TOKEN, QUESTS_TG_USER_IDS; прокси на хосте :12334
# QUESTS_WHISPER_MODEL=small|medium|large (default small — medium ~1 мин/сек на слабом CPU)
# GHCR: QUESTS_API_IMAGE=ghcr.io/<owner>/quests-api:main  (и BOT)

# --env-file обязателен: иначе ${QUESTS_*_IMAGE} не подхватятся из корневого .env
docker compose --env-file .env -f deploy/docker/docker-compose.yml up -d --build

curl -sS http://127.0.0.1:8765/api/health
curl -sS http://127.0.0.1:8080/api/health
```

Логи: `docker compose --env-file .env -f deploy/docker/docker-compose.yml logs -f`

### CI / GHCR (ветка `main`)

GitHub Actions (`.github/workflows/main.yml`, workflow **CI**):

1. `pytest` + `go build`/`go test` на push/PR
2. Path-filter: `api` (+ frontend SPA) и узкий `bot` (telegram/llm/stt + deps)
3. Job **`build-images`**: `docker buildx bake` (`deploy/docker/docker-bake.hcl`) —
   если нужны оба target’а, `python-base` собирается один раз → общие layer digests на GHCR
4. `build-images` идёт параллельно с тестами и пушит только `ghcr.io/<owner>/quests-*:<sha>`
   (BuildKit cache `scope=quests`); тег `:main` не трогает
5. Job **`promote`** (после зелёных `test` и `build-images`) — registry-side ретег `:<sha>` → `:main`
   (`imagetools create`, без пересборки). При красных тестах `:main` не двигается; осиротевшие `:<sha>`
   чистятся retention-правилом пакета
6. Job **`deploy`** (только push на `main`, после успешного `promote`) — раннер сам живёт на
   проде (self-hosted, не `ubuntu-latest`: облачный раннер не видит приватный IP сервера), гоняет
   `git reset --hard origin/main` + `compose pull` + `compose up -d` —
   [`deploy/docker/ci-deploy.sh`](ci-deploy.sh). Настройка: [`docs/deploy-ssh.md`](../../docs/deploy-ssh.md).

На сервере в `.env`:

```bash
QUESTS_API_IMAGE=ghcr.io/<owner>/quests-api:main
QUESTS_BOT_IMAGE=ghcr.io/<owner>/quests-bot:main
```

Один раз: `echo $CR_PAT | docker login ghcr.io -u USER --password-stdin`
(пакеты GitHub часто private — нужен PAT с `read:packages`).

Ручной прогон (без ожидания CI):

```bash
docker compose --env-file .env -f deploy/docker/docker-compose.yml pull
docker compose --env-file .env -f deploy/docker/docker-compose.yml up -d
```

(`env_file:` в compose кормит контейнеры; `--env-file` нужен для подстановки `image: ${QUESTS_*_IMAGE}`.)

Ручной прогон сборки: Actions → **CI** → Run workflow (`force_api` / `force_bot`) — деплой всё
равно подхватит новый образ на следующем push в `main`, либо гони `ci-deploy.sh` руками на сервере.

Локально по-прежнему: без `QUESTS_*_IMAGE` → `quests-*:local` и `up --build`.

### Прокси для бота

Бот в `network_mode: host`, чтобы достучаться до прокси на `127.0.0.1`
(типичный sing-box / clash). Дефолт: `http://127.0.0.1:12334`.

Если прокси слушает только loopback — **не** используй `host.docker.internal`:
из контейнера в bridge-сети пакет приходит на docker0, а не на `127.0.0.1` →
connection refused.

Альтернатива без host-network: слушать прокси на `0.0.0.0:12334` и
`QUESTS_TG_PROXY=http://host.docker.internal:12334` (убери `network_mode: host`,
верни `QUESTS_API=http://api:8765`).

### Прокси для `api` (ассистент «Команда» → Groq)

Кнопка «Команда» в веб-UI (текст → квесты) — это чистый Go внутри
`quests-server`, он сам ходит в Groq. `api` сидит на обычной bridge-сети (не
`host`), поэтому в compose ему заданы `extra_hosts: host.docker.internal:host-gateway`
и `QUESTS_LLM_PROXY=http://host.docker.internal:12334` (override через `.env`,
как и `QUESTS_TG_PROXY`). Тем самым применима та же оговорка про loopback-only
прокси, что и у бота — прокси должен слушать `0.0.0.0:12334`, а не только
`127.0.0.1`, иначе `host.docker.internal` до него не достучится.

**Важно:** это структурная правка контейнера (не только образ) — Watchtower
её не применит, он лишь подменяет образ у уже существующего контейнера с его
старым конфигом. После обновления `docker-compose.yml` нужен реальный
`docker compose up -d` (или `--force-recreate`) на актуальном чекауте, иначе
`extra_hosts` останется пустым и запросы в Groq полетят напрямую (Cloudflare
забанит по региону, HTTP 403).

## HUD (не Docker)

```bash
export QUESTS_API=http://SERVER_IP:8765
just run-overlay
# или: systemctl --user enable --now quests-overlay.service
```

## Данные

PostgreSQL — volume `quests-pg` (сервис `postgres` в основном compose, нужен `POSTGRES_PASSWORD`), см. `docs/postgres.md`; файлы приложения — volume `quests-data` (`/app/data`).

## Переменные

См. `.env.example`. В compose:

- `QUESTS_HOST=0.0.0.0`
- `QUESTS_API` у бота: `http://127.0.0.1:8765` (host network)
- `QUESTS_RELOAD=0`
- `QUESTS_API_IMAGE` / `QUESTS_BOT_IMAGE` — опционально GHCR
- `GROQ_API_KEY` (или `QUESTS_GROQ_API_KEY`) — нужен `api` для кнопки «Команда»; без ключа сама кнопка просто вернёт ошибку, остальной UI не пострадает
- `QUESTS_WEBDAV_URL` / `QUESTS_WEBDAV_USER` / `QUESTS_WEBDAV_PASS` — внешний WebDAV (не в этом compose). Пустой URL = загрузка вложений выключена.
- `QUESTS_CLAMAV_ADDR` — по умолчанию `clamav:3310`. Первый старт clamd качает сигнатуры; до готовности загрузки отвечают 503.

WebDAV — отдельная инфра на хосте (сейчас `<lan-ip>:8082`), не сервис этого compose. Из контейнера `api` на тот же LAN-IP ходить можно как есть; если WebDAV на том же боксе, что и compose — `http://host.docker.internal:8082`.
