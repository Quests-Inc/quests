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
- **Секреты** — `deploy/prod/secrets.enc.env`, зашифрованы SOPS (age); открывает их doco-cd ключом с хоста.
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

1. **Секреты приложения.** `just secrets-init` (создаёт age-ключ в `~/.config/sops/age/keys.txt`, кладёт публичный
   ключ в `deploy/prod/.sops.yaml`, шифрует шаблон), затем `just secrets-edit` — заполнить значения
   (`POSTGRES_PASSWORD` — **тот же**, что сейчас на проде, иначе api не подключится к существующей БД;
   `QUESTS_SECRET_KEY` — тот же, что сейчас, иначе запечатанные секреты не откроются). Не обязательные
   несекретные настройки — `deploy/prod/config.env`. Закоммитить `.sops.yaml` и `secrets.enc.env`.
2. **GitHub → Settings → Environments → `production`**: Required reviewers — вы.
3. **GitHub → Secrets (`production` или репозиторий):**
   - `SOPS_AGE_KEY` — приватный ключ age (содержимое строки `AGE-SECRET-KEY-…` из файла ключа);
   - `DEPLOY_GIT_TOKEN` — fine-grained PAT, только чтение (*Contents: read*) этого репозитория;
   - `GHCR_PULL_TOKEN` — PAT с `read:packages` (образы приватные);
   - `TG_BOT_TOKEN` — необязательно, пока пусто — уведомлений нет.
   **Variables:** `GHCR_USER` (логин для ghcr.io), `TG_CHAT_ID` (с токеном), `DEPLOY_STATE_DIR`
   (по умолчанию `/srv/quests-deploy`; каталог должен быть доступен раннеру), `DOCO_CD_VERSION` (по умолчанию `latest`: закрепите после первого запуска).
4. **Раннер на сервере** (временно): Settings → Actions → Runners → New self-hosted runner, под отдельным пользователем,
   состоящим в группе `docker`. Репозиторий должен оставаться **приватным**: self-hosted раннер исполняет то,
   что говорят workflow-ы (см. предупреждение в `bootstrap-deploy.yml`).
5. **Первый релиз**: `just release 0.1.0` (или версия, с которой начинаете). Он соберёт образы и создаст ветку `prod`.
6. Actions → *Bootstrap deploy agent* → Run workflow: запустит doco-cd, тот увидит `prod` и поднимет стек.

Проверить: `quests-server version` / `/api/health` показывают версию; в логах doco-cd — «deployment completed».

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
- пустые `APPRISE_*` не требуются: без токена бота `notify.env` вообще не создаётся.
