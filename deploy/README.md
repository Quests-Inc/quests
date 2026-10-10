# Релизы и деплой

Приложение собирается здесь, **прод описан в отдельном репозитории**
[Quests-Inc/quests-deploy](https://github.com/Quests-Inc/quests-deploy) (compose, настройки, секреты, doco-cd). Здесь
остаётся только то, что нужно, чтобы выпустить образы и объявить релиз.

```
 gh release create v1.4.0   (или `just release 1.4.0`)
   └─ Release workflow: проверки → образы ghcr.io/quests-inc/quests-{api,bot,stt}:1.4.0
   └─ pull request в quests-deploy: «Release v1.4.0» (version.env)
        └─ вы его вливаете ──► main ◄── doco-cd на сервере поллит раз в минуту ──► docker compose up -d
```

- **Версии — SemVer** (`MAJOR.MINOR.PATCH`), тег `vX.Y.Z` создаёт публикация GitHub Release из `main`. Версия вшита в бинарь
  (`quests-server version`, `api.version` в `/api/health`) и в теги образов. Пререлизы (`-rc.1`) собирают образы, но PR на деплой не открывают.
- **Выкат — это слитый PR** в `quests-deploy`: это то самое явное действие (на бесплатном тарифе обязательное ревью
  приватного репозитория недоступно, так что это договорённость, а не запрет). Откат — revert этого коммита или PR со старой
  версией (или ручной запуск Release workflow со старой версией: он откроет такой PR). Образы неизменяемы.
- **Приложение не знает про прод**: ни секретов, ни раннера, ни SOPS-ключа здесь нет — только токен `DEPLOY_REPO_TOKEN` с правом
  записи в `quests-deploy` (PR) и временный self-hosted раннер для первого запуска агента (см. ниже).
- **Миграции схемы** сервер делает сам при старте, предварительно сняв `pg_dump` (`data/backups/pre-migrate-…`). Откат возможен,
  пока данные подходят старой схеме: колонки добавляем в одном релизе, убираем в следующем.

## Выпуск релиза

```bash
just release 1.4.0        # проверит, что CI на main зелёный, и создаст GitHub Release v1.4.0 с авто-заметками
```

Пререлиз: `just release 1.5.0-rc.1`. После публикации Release workflow сам соберёт образы и откроет PR в `quests-deploy`.

## Что нужно настроить в этом репозитории (один раз)

| Имя | Тип | Зачем | Где взять |
|---|---|---|---|
| `DEPLOY_REPO_TOKEN` | secret | Release workflow открывает PR в `quests-deploy` | *Settings → Developer settings → Fine-grained tokens*: владелец ресурса — организация `Quests-Inc` (организация должна разрешать fine-grained токены), репозиторий только `quests-deploy`, права *Contents: Read and write* + *Pull requests: Read and write* |
| `DEPLOY_REPO` | variable | если репозиторий деплоя называется иначе | по умолчанию `Quests-Inc/quests-deploy` |

Для одноразового *Bootstrap deploy agent* (запускает doco-cd на сервере через временный self-hosted раннер этого
репозитория) нужны ещё `DEPLOY_GIT_TOKEN`, `GHCR_PULL_TOKEN`, `GHCR_USER` и по желанию `LEGACY_ENV*`, `SOPS_AGE_KEY`, `TG_*`: их
описание и способ получения — в README репозитория `quests-deploy`. Как только на сервере появится shell, bootstrap запускается
руками (`doco-cd/bootstrap.sh` оттуда), а раннер и эти секреты здесь можно удалить.

## Прокси для бота и ассистента

Бот работает в `network_mode: host`, чтобы достучаться до прокси на `127.0.0.1` (типичный sing-box / clash), по умолчанию
`http://127.0.0.1:12334`. Если прокси слушает только loopback, **не** используйте `host.docker.internal`: из bridge-сети пакет
приходит на docker0, а не на `127.0.0.1`. `api` (кнопка «Команда» → Groq) сидит на bridge-сети, ему заданы
`extra_hosts: host.docker.internal:host-gateway` и `QUESTS_LLM_PROXY=http://host.docker.internal:12334` (переопределяется в
`config.env` репозитория деплоя); прокси должен слушать `0.0.0.0:12334`.
