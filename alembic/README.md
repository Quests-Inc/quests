# Alembic — архив

Цепочки миграций Alembic **больше не запускаются**: схему ведёт Go (`quests-server migrate`,
SQL-файлы в `go/internal/db/sql/`, см. `docs/postgres.md`).

Каталог оставлен как история:

- `versions/` — старая SQLite-цепочка, заморожена на `1a2b3c4d5e6f`;
- `versions_pg/` — PostgreSQL-цепочка `1a2b3c4d5e6f` … `f5b7d9e1a3c6`. Её SQL перенесён в Go без изменений
  (`go test ./internal/db` сверяет копию с этим архивом); новые ревизии сюда **не добавляются**.

Запустить архив при необходимости: `uv run --group archive alembic upgrade head` (нужен `alembic.ini` из корня).
