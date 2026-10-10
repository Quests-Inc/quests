# deploy/docker — сборка образов и разработка

Здесь то, что нужно **сборке**; как образы попадают на прод — в [`../README.md`](../README.md).

| Файл | Что |
|---|---|
| `Dockerfile` | цели `api` (Go-бинарь + SPA, `debian:bookworm-slim`), `bot`, `stt` |
| `docker-bake.hcl` | сборка всех целей разом (`just` не нужен: `docker buildx bake -f deploy/docker/docker-bake.hcl`); теги — `TAG` (версия релиза), версия в бинаре — `VERSION` |
| `docker-compose.dev.yml` | локальный API с собственным PostgreSQL (`just dev-api`, `just dev-db`) |
| `entrypoint-bot.sh`, `entrypoint-stt.sh` | точки входа Python-образов |
| `backup-monitoring.sh`, `disk-guard.sh` | cron-скрипты обслуживания хоста |

Локально:

```bash
docker buildx bake -f deploy/docker/docker-bake.hcl api   # → quests-api:local
just dev-api                                              # API + PostgreSQL в Docker на :8765
```

## Образ `api`

Go-бинарь `quests-server` + собранный SPA + CLI `quests` (для хук-скриптов оператора) + `pg_dump`.
Миграции схемы сервер делает сам при старте (перед этим — `pg_dump`, если есть что применять), пользовательские
команды он не исполняет: они идут на раннеры (`quests-runner`). Интерпретаторов в образе нет.

## HUD (не Docker)

```bash
export QUESTS_API=http://SERVER_IP:8765
just run-overlay
# или: systemctl --user enable --now quests-overlay.service
```
