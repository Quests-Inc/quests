# Accounts and access

Quests is a closed-access instance. **Accounts gate the whole API**, and data is split into **workspaces**: every
account belongs to one or more isolated workspaces and sees only their data. Inside a workspace a person is either an
**owner** (manages commands, secrets, runners, people, renaming, deleting and exporting the workspace) or a **member**
(reads and writes the workspace's quests, notes, attachments and tags). The full matrix of who can do what, and where each
rule is enforced, is in [`security.md`](security.md); the workspace mechanics (row-level security, invitations, soft
delete, export/import) are in [`postgres.md`](postgres.md).

A fresh local database with no accounts keeps working as before: `127.0.0.1`, no login.

## How it is switched on

The mode is set by `QUESTS_AUTH`:

| Value | Behaviour |
|---|---|
| `auto` (default) | Auth turns on as soon as the database holds at least one account |
| `on` | Auth is mandatory; the server does not start without accounts |
| `off` | Auth is off — allowed **only** when bound to loopback |

A separate safeguard: the server **refuses to start** when it is bound to anything but
loopback and there are no accounts. You cannot forget to lock an instance before exposing it — the process simply will not come up.

## Creating an account

These commands go straight to the database (`QUESTS_DATABASE_URL`), not over HTTP — run them where the
database is reachable (in Docker: `docker compose exec api quests-server …`).

```bash
quests-server useradd <username>
```

The first account gets workspace 1 (the migrated data); every later one gets a new personal workspace with the default
categories. The password is asked for interactively and not echoed. It is never passed through argv or env — it would end up in
the shell history and in `/proc`. At least 8 characters; logins are `[a-z0-9._-]{2,32}`.

```bash
quests-server users              # list accounts (and who has 2FA)
quests-server passwd <username>  # change a password
```

In Docker:

```bash
docker exec -it quests-api quests-server useradd <username>
```

Restart the server after creating the first account.

Further people join through an **invitation link** (below) rather than through the CLI.

## Workspaces, roles, invitations

An account can belong to several workspaces. A browser session works in one at a time (Settings → «Пространства» (Workspaces));
an API token is pinned to the workspace it was minted for: `quests-server token add <name> [user] --workspace <id>`.

- **Roles.** `owner` and `member`; the last owner cannot leave. Members have full access to the workspace's data; owners
  also manage people, commands, secrets and runners.
- **Invitations.** An owner mints a one-time link (`/?invite=<token>`, valid 7 days, only its hash is stored) in
  any workspace. Opening it: a signed-in account joins; otherwise the page signs a new account up (login + password) that
  then joins. This is the only self-service sign-up.
- **Deleting a workspace** is a soft delete: it disappears at once (its tokens, runners and sessions stop) and is purged after
  7 days; `quests-server workspace restore <id>` undoes it before that, `workspace ls` lists pending purges. The main
  workspace (id 1) cannot be deleted.
- **Export / import.** Owners export a workspace as a zip (Settings → Пространства → «Архив»); anyone can import an
  archive, which always creates a new workspace owned by the importer.
- **Quota.** Attachments are limited per workspace (`QUESTS_WORKSPACE_QUOTA_MB`, default 2048 MB) and per file (`QUESTS_MAX_UPLOAD_MB`, default 25 MB).
- **Host admin commands.** `quests-server workspace ls|add|member|restore`, `runner add|ls|rm`, `token add|ls|rm`, `2fa-reset`
  (see [`cli.md`](cli.md) and the admin guide of your deployment).

## Tokens for headless clients

The browser uses a session cookie, but the CLI, the HUD overlay, the Telegram bot and MCP are
headless. They need a long-lived bearer token.

```bash
quests-server token add overlay     # shows the secret ONCE
quests-server token ls
quests-server token rm <id>
```

Only the SHA-256 hash of a token is stored — a leaked copy of the database yields no working
tokens. The secret reaches a client through the environment:

```bash
QUESTS_API_TOKEN=<secret>
```

Every client reads that variable: `quests` (the Go CLI), the overlay, the bot, the MCP server.

### Tokens from the web interface

Settings → «Подключения» (Connections): a signed-in user creates tokens for their own clients (HUD, MCP, Android, CLI, bot), pinned to the workspace
they are in, and revokes them. The secret is shown once (only its hash is stored); at most 25 live tokens per account. Only a browser
session can manage tokens — a token cannot mint or revoke tokens, so a leaked one stays contained until it is revoked.
`quests-server token add|ls|rm` on the host still works for the host admin.

## What is closed, what is open

Open without authentication:

- `GET /api/ping` — liveness for monitoring and the Docker healthcheck
- `GET /api/auth/state` — tells the SPA whether to draw the login form
- `POST /api/auth/login`, `POST /api/auth/login/mfa` (the second step of a login, see below)
- an open invitation link and its sign-up (`GET|POST /api/invites/{token}[/accept]`)
- the SPA's static files — otherwise there would be nothing to draw the login screen with

Everything else, including `/api/health`, the SSE stream `/api/events` and the WebSocket `/ws`,
needs a cookie or a token.

## How it works inside

- **Passwords**: PBKDF2-HMAC-SHA256, 600,000 iterations, a random 16-byte salt,
  encoded as `pbkdf2_sha256$<iters>$<salt_b64>$<hash_b64>`.
- **Sessions**: an opaque secret (32 random bytes); the database holds its SHA-256.
  They live for 30 days in the `usersession` table, so they are revoked on the server.
  The cookie `quests_session` is `HttpOnly`, `SameSite=Lax`, and `Secure` when the request came over TLS (or through a
  proxy that says `X-Forwarded-Proto: https`) or with `QUESTS_SECURE_COOKIES=1`.
- **CSRF**: `SameSite=Lax` plus an `Origin` check for cookie requests —
  a foreign origin gets 403. Tokens are not subject to that check: they cannot be
  slipped in from a browser unnoticed by the owner.
- **Login guessing**: a non-existent user is run through a dummy
  hash so that the response time does not reveal whether an account exists; there is a per-IP throttle and a stall after repeated
  failures for a login (see [`security.md`](security.md)).

## Two-factor login (TOTP)

Enabled in Settings → «Аккаунт» (Account): a QR code (or the key by hand) for an authenticator app, confirmation with
six digits, then ten **recovery codes** (shown once, each works once).

- Login: password → if the account has 2FA, a one-time “ticket” (5 minutes, 5 attempts) is issued instead of a session,
  and it is exchanged together with a code for a session (`POST /api/auth/login/mfa`). A code from the app is accepted once
  (a repeat of the same 30-second step is rejected); one step back and forward is allowed for clock drift.
- The TOTP secret is stored encrypted (the same server key as the assistant's keys), recovery codes only as
  hashes. Enabling ends the account's other sessions; disabling and reissuing codes require a valid code.
- API tokens (HUD, CLI, bot, MCP) are issued from a browser session and need no second factor.
- Phone and codes lost: on the host `docker exec quests-api quests-server 2fa-reset <login>` removes 2FA and closes the
  account's sessions (`quests-server users` shows who has it on).

## HTTPS

The session cookie is marked `Secure` whenever the request arrived over TLS or through a proxy that sets
`X-Forwarded-Proto: https` (the edge gateway does). With `QUESTS_SECURE_COOKIES=1` it is `Secure` always, and a browser
drops it over plain http — turn that on only when the instance is reached through TLS exclusively.
