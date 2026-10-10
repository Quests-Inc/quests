# Релизы и деплой

Прод обновляется **только** явным релизом. Схема:

```
 gh release create v1.4.0 ──► Release workflow ──► образы ghcr.io/<owner>/quests-{api,bot,stt}:1.4.0
   (или `just release 1.4.0`)        │
                                     └─ job deploy (ждёт подтверждения в environment `production`)
                                          └─ deploy/prod/publish.sh ──► ветка `prod`: коммит «release v1.4.0»
                                                                          (compose + version.env + secrets.enc.env)
                                                                               ▲
 сервер (без белого IP)  ◄── doco-cd поллит `prod` раз в минуту ───────────────┘  → `docker compose up -d`
```

- **Версии — SemVer** (`MAJOR.MINOR.PATCH`), тег `vX.Y.Z` создаёт публикация GitHub Release из `main`.
  Версия вшита в бинарь (`quests-server version`, `api.version` в `/api/health`) и в теги образов.
  `-rc.1` и прочие пререлизы собирают образы, но **не** деплоятся.
- **Сервер сам ничего не ждёт извне**: doco-cd ходит в репозиторий исходящими запросами, входящих портов
  и белого IP не нужно.
- **Откат** — Actions → *Release* → *Run workflow* с прежней версией (`1.3.2`): образы уже есть, двигается
  только `prod`. Схему БД откат не возвращает: откатывать можно, пока данные ей соответствуют (добавляем
  колонки в одном релизе, убираем в следующем). Перед каждой миграцией сервер сам делает `pg_dump`
  (`pre-migrate-….dump` в `data/backups/`).
- **Секреты** — два источника, можно любой или оба (SOPS главнее там, где переменная задана и там, и там):
  1. **Старый `.env` хоста** (запасной путь, работает сразу): bootstrap берёт его с раннера (файл прежнего деплоя,
     `/home/amarant/Documents/projects/quests/.env`, или другой путь из `LEGACY_ENV_FILE`) либо из секрета `LEGACY_ENV`,
     кладёт в `$DEPLOY_STATE/legacy.env`, а doco-cd монтирует его как `/legacy.env`; compose читает его как необязательный
     `env_file`. Секреты нигде не подставляются через `${…}`: контейнеры сами читают env-файлы.
  2. **SOPS** — `deploy/prod/secrets.enc.env` (age), лежит в репозитории зашифрованным, открывает его doco-cd ключом с хоста.
     Включится, как только появится файл и ключ; до тех пор его просто нет.
- **Уведомления** об итогах деплоя — в Telegram через Apprise; пока токена бота нет, они молчат.

## Выпуск релиза

```bash
just release 1.4.0        # проверит, что CI на main зелёный, и создаст GitHub Release v1.4.0 с авто-заметками
```

Дальше всё само: проверки → образы → job `deploy` ждёт одобрения (*Settings → Environments → production →
Required reviewers*) → `prod` двигается → doco-cd выкатывает. Пререлиз: `just release 1.5.0-rc.1`.

## Первая установка (один раз)

Сейчас доступа на сервер нет, поэтому doco-cd ставится workflow-ом на временном self-hosted раннере
(`.github/workflows/bootstrap-deploy.yml`). Как только на сервере появится shell, раннер можно удалить, а
`deploy/doco-cd/bootstrap.sh` запускать руками.

Минимум (секреты берутся из старого `.env` сервера, SOPS не нужен):

1. **GitHub → Settings → Environments → New environment → `production`**, включить *Required reviewers* (вы).
2. **Секреты и переменные** (*Settings → Secrets and variables → Actions*; удобнее положить в environment `production`:
   тогда они видны только деплой-job-ам). Что и откуда брать — в таблице ниже.
3. **Раннер на сервере** (временно): *Settings → Actions → Runners → New self-hosted runner*, под отдельным
   пользователем из группы `docker`, с доступом на чтение к файлу старого `.env`. Репозиторий должен оставаться
   **приватным**: self-hosted раннер исполняет то, что говорят workflow-ы (предупреждение в `bootstrap-deploy.yml`).
4. **Первый релиз**: `just release 0.1.0` (или версия, с которой начинаете): соберёт образы и создаст ветку `prod`
   после вашего одобрения job `deploy`.
5. *Actions → Bootstrap deploy agent → Run workflow*: запустит doco-cd; тот увидит `prod` и поднимет стек
   поверх существующих томов и БД.

Проверить: `/api/health` показывает версию; в логах doco-cd — «deployment completed».

Позже, когда захотите SOPS: `just secrets-init`, `just secrets-edit`, закоммитить `.sops.yaml` и `secrets.enc.env`, добавить
секрет `SOPS_AGE_KEY` и перезапустить Bootstrap. Старый `.env` после этого можно убрать.

### Секреты и переменные GitHub: где взять

| Имя | Тип | Обязательно | Где взять |
|---|---|---|---|
| `DEPLOY_GIT_TOKEN` | secret | да | GitHub → ваш профиль → *Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token*: *Repository access* → *Only select repositories* → этот репозиторий; *Permissions → Repository → Contents: Read-only*. Срок до года, потом перевыпустить. doco-cd читает с него ветку `prod` |
| `GHCR_PULL_TOKEN` | secret | да | *Developer settings → Personal access tokens → Tokens (classic) → Generate*: единственная галка `read:packages` (fine-grained токены пакеты не умеют). Нужен серверу, чтобы тянуть приватные образы из ghcr.io |
| `GHCR_USER` | variable | да | логин GitHub-аккаунта, которому принадлежит токен выше |
| `LEGACY_ENV` | secret | нет | содержимое старого `.env` целиком, если раннер не может прочитать его с диска. Иначе путь на хосте раннера в `LEGACY_ENV_FILE` (по умолчанию `/home/amarant/Documents/projects/quests/.env`) |
| `LEGACY_ENV_FILE` | variable | нет | путь к старому `.env` на хосте раннера, если он не по умолчанию |
| `SOPS_AGE_KEY` | secret | нет (с SOPS — да) | строка `AGE-SECRET-KEY-…` из файла ключа (`~/.config/sops/age/keys.txt`, создаётся `just secrets-init`) |
| `TG_BOT_TOKEN` + `TG_CHAT_ID` | secret + variable | нет | токен у @BotFather (`/newbot`), id чата — написать боту и открыть `https://api.telegram.org/bot<токен>/getUpdates`, поле `chat.id`. Пока пусто, уведомлений нет |
| `DEPLOY_STATE_DIR` | variable | нет | каталог состояния на хосте, по умолчанию `/srv/quests-deploy` (раннер должен уметь в него писать) |
| `DOCO_CD_VERSION` | variable | нет | тег образа doco-cd (по умолчанию `latest`; закрепите после первого запуска) |

`GITHUB_TOKEN` для публикации образов и ветки `prod` выдаёт сам GitHub, получать ничего не нужно.

## Что где лежит

| Путь | Что |
|---|---|
| `deploy/docker/` | то, что нужно **сборке**: `Dockerfile`, `docker-bake.hcl`, entrypoint-ы бота и STT, dev-compose, скрипты обслуживания |
| `deploy/prod/` | всё, что доезжает до прода: `docker-compose.yml`, `docker-compose.monitoring.yml`, `.doco-cd.yml`, `config.env`, `secrets.enc.env`, `nginx.conf`, `monitoring/`, `publish.sh` |
| `deploy/doco-cd/` | агент деплоя и `bootstrap.sh` |
| ветка `prod` | сгенерированная: содержимое `deploy/prod/` + `version.env`, один коммит на релиз, без исходников приложения |

Содержимое `deploy/prod/` уже организовано как отдельный репозиторий: когда репозитории разделят, достаточно
перенести каталог (и `deploy/doco-cd/`), а `publish.sh` из шага release превратить в «закоммитить версию
в deploy-репозиторий».

## Мониторинг

`docker-compose.monitoring.yml` (Uptime Kuma, Prometheus, Grafana, exporters) входит в тот же деплой; чтобы выключить,
уберите его из `compose_files` в `deploy/prod/.doco-cd.yml`. `remove_orphans: false`, чтобы doco-cd никогда не
удалял контейнеры, которых не знает. Подробности — `docs/monitoring.md`.

## Прокси для бота и ассистента

Бот работает в `network_mode: host`, чтобы достучаться до прокси на `127.0.0.1` (типичный sing-box / clash),
по умолчанию `http://127.0.0.1:12334`. Если прокси слушает только loopback, **не** используйте
`host.docker.internal`: из bridge-сети пакет приходит на docker0, а не на `127.0.0.1`.

`api` (кнопка «Команда» → Groq) сидит на bridge-сети, ему заданы `extra_hosts: host.docker.internal:host-gateway`
и `QUESTS_LLM_PROXY=http://host.docker.internal:12334` (переопределяется в `config.env`); прокси должен слушать
`0.0.0.0:12334`.

## Что ещё не проверено на живом сервере

Этот поток собран без доступа на сервер. При первом запуске стоит убедиться, что:
- doco-cd видит ключ age и токен из docker-секретов (`/run/secrets/…`) и умеет `compose pull` из приватного ghcr.io
  с `DOCKER_CONFIG=/docker-config`;
- существующие тома (`quests_quests-data`, `quests_quests-pg`, `quests_clamav-sigs`) подхвачены: проект называется `quests`;
- `/legacy.env` читается как необязательный `env_file` из контейнера doco-cd (путь в compose абсолютный) и перекрывается `config.env` и `secrets.enc.env`;
- без токена бота и без ключа age `doco.env` не создаётся: doco-cd стартует без `APPRISE_*` и `SOPS_AGE_KEY_FILE`.
