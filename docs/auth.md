# Аккаунты и доступ

Quests — однопользовательский инстанс с закрытым доступом. Аккаунты **гейтят
доступ ко всему API**, но не разделяют данные: журнал общий
для всех аккаунтов. Мультитенантности нет.

## Как это включается

Режим задаётся `QUESTS_AUTH`:

| Значение | Поведение |
|---|---|
| `auto` (по умолчанию) | Auth включается, как только в БД появился хоть один аккаунт |
| `on` | Auth обязателен; без аккаунтов сервер не стартует |
| `off` | Auth выключен — разрешено **только** при биндe на loopback |

Отдельная страховка: сервер **отказывается стартовать**, если бинд не на
loopback, а аккаунтов нет. Забыть закрыть инстанс перед выставлением наружу
нельзя — процесс просто не поднимется.

Чистая локальная БД без аккаунтов работает как раньше: `127.0.0.1`, без логина.

## Создание аккаунта

Команды идут напрямую в базу (`QUESTS_DATABASE_URL`), без HTTP — их надо выполнять там,
откуда база доступна (в Docker: `docker compose exec api quests-server …`).

```bash
quests-server useradd <username>
```

Пароль спрашивается интерактивно и не эхоится. Через argv или env пароль не
передаётся никогда — иначе он осядет в history и в `/proc`. Минимум 8 символов.

```bash
quests-server users              # список аккаунтов
quests-server passwd <username>  # сменить пароль
```

В Docker:

```bash
docker exec -it quests-api quests-server useradd <username>
```

После создания первого аккаунта сервер нужно перезапустить.

## Токены для headless-клиентов

Браузер ходит по сессионной куке, но CLI, HUD-оверлей, Telegram-бот и MCP —
безголовые. Им нужен долгоживущий bearer-токен.

```bash
quests-server token add overlay     # покажет секрет ОДИН раз
quests-server token ls
quests-server token rm <id>
```

В БД лежит только SHA-256 хеш токена — утёкшая копия базы не даёт рабочих
токенов. Секрет отдаётся клиенту через env:

```bash
QUESTS_API_TOKEN=<секрет>
```

Читают эту переменную все клиенты: `quests` (Go CLI), оверлей, бот, MCP-сервер.

## Что закрыто, что открыто

Открыты без аутентификации:

- `GET /api/ping` — liveness для мониторинга и docker healthcheck
- `GET /api/auth/state` — говорит SPA, рисовать ли форму логина
- `POST /api/auth/login`, `POST /api/auth/login/mfa` (второй шаг входа, см. ниже)
- статика SPA — иначе экран логина было бы нечем отрисовать

Всё остальное, включая `/api/health`, SSE `/api/events` и WebSocket `/ws`,
требует куки или токена.

## Как устроено внутри

- **Пароли**: PBKDF2-HMAC-SHA256, 600 000 итераций, случайная 16-байтная соль,
  кодировка `pbkdf2_sha256$<iters>$<salt_b64>$<hash_b64>`.
- **Сессии**: непрозрачный секрет (32 случайных байта), в БД — его SHA-256.
  Живут 30 дней, лежат в таблице `usersession`, поэтому отзываются на сервере.
  Кука `quests_session`: `HttpOnly`, `SameSite=Lax`, `Secure` при
  `QUESTS_SECURE_COOKIES=1`.
- **CSRF**: `SameSite=Lax` плюс проверка `Origin` для запросов по куке —
  чужой origin получает 403. Токены этой проверке не подлежат: их нельзя
  подставить из браузера незаметно для владельца.
- **Перебор логинов**: несуществующий пользователь прогоняется через фиктивный
  хеш, чтобы время ответа не выдавало, существует ли аккаунт.

## Двухфакторный вход (TOTP)

Включается в «Настройки → Аккаунт»: QR-код (или ключ вручную) в приложение-аутентификатор, подтверждение
шестью цифрами, затем десять **кодов восстановления** (показываются один раз, каждый подходит однократно).
Только при PostgreSQL.

- Вход: пароль → если у аккаунта включена 2FA, вместо сессии выдаётся одноразовый «билет» (5 минут, 5 попыток),
  его меняют вместе с кодом на сессию (`POST /api/auth/login/mfa`). Код из приложения принимается один раз
  (повтор того же 30-секундного шага отклоняется), допускается шаг назад и вперёд на расхождение часов.
- Секрет TOTP хранится зашифрованным (тот же ключ сервера, что у ключей ассистента), коды восстановления — только
  хэшами. Включение завершает остальные сессии аккаунта; отключение и перевыпуск кодов требуют действующий код.
- API-токены (HUD, CLI, бот, MCP) выдаются из браузерной сессии и второго фактора не требуют.
- Потерян телефон и коды: на хосте `docker exec quests-api quests-server 2fa-reset <логин>` — снимает 2FA и закрывает
  сессии аккаунта (`quests-server users` показывает, у кого она включена).

## HTTPS

При `QUESTS_SECURE_COOKIES=1` кука помечается `Secure` и по обычному http
браузер её отбросит — включать только вместе с реальным TLS.

## Workspaces

On PostgreSQL an account can belong to several isolated workspaces (see `docs/postgres.md`).
A browser session works in one at a time (Settings → Пространства); an API token is pinned to the
workspace it was minted for: `quests-server token add <name> [user] --workspace <id>`.

## Tokens from the web interface

Settings → Подключения: the signed-in user creates tokens for their own clients (HUD, MCP, Android, CLI, bot), pinned to the workspace
they are in, and revokes them. The secret is shown once (only its hash is stored); at most 25 live tokens per account. Only a browser
session can manage tokens — a token cannot mint or revoke tokens, so a leaked one stays contained until it is revoked.
`quests-server token add|ls|rm` on the host still works for the host admin.
