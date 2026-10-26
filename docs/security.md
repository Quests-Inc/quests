# Security model (accounts, workspaces, invited members)

Who can do what, what is enforced where, and how to re-check it.

## Trust boundaries

| Who | Can | Cannot |
|---|---|---|
| Anyone (no account) | `/api/ping`, login (throttled), open an invitation link and sign up through it | anything else |
| Member of a workspace | read and write that workspace's quests, notes, attachments, tags; make API tokens for it; see the server health chips; export nothing | set commands, secrets, runners; invite; rename/delete/export; see other workspaces |
| Owner of a workspace | everything above + commands (for runners), secrets, runners, event hooks, invitations, rename, delete (not the main workspace), export | — |
| Host admin | `quests-server` admin CLI (accounts, tokens, backups, workspace restore) | — |

**The server executes no user shell.** Check, emit and hook Code nodes run on runners (`quests-runner` on any device, the
server's own host included): a runner sits where its owner put it, with the rights of the user it runs as. Trusted
`sink.http` / `sink.socket` may dial from the API process after host checks. There is no "trusted workspace" and no
operator role in the web interface. The full rule set is in [`automation.md`](automation.md).

## Where each rule lives

- **Row isolation** — PostgreSQL row-level security (`workspace_id = app.workspace_id`), the API role `quests_app` has no
  BYPASSRLS. A request without a workspace sees nothing. Tests: `go/internal/platform/db` (`QUESTS_TEST_PG_URL=…`).
- **Workspace per request** — `withWorkspace` in `go/internal/httpapi/workspaces.go`: sessions/tokens are pinned, the
  `X-Quests-Workspace` header is checked against membership. The assistant's loopback token must name the workspace
  (it used to fall into the default one — an invited user could rewrite the owner's quests through it).
- **Commands** — `kernel.Commands.Check` (only owners write a command, editing around an existing one is fine; a command needs a runner), in `go/internal/kernel/commands.go`. A command stored without a runner
  is never run: steps stay idle, notes and templates report "choose a runner".
  Secrets names that would bend how a process starts (`PATH`, `LD_*`, `BASH_ENV`, `QUESTS_*`…) are refused and never exported.
- **Exports** — PDF is rendered in-process by `go/internal/pdfdoc` from a document the client describes (title, markdown, steps):
  no HTML or CSS is interpreted, nothing is fetched from the network or the disk, and the only images are attachments the
  server loads itself under the caller's workspace.
- **Browser** — CSP, `X-Frame-Options`, `Referrer-Policy: no-referrer` (invitation secrets sit in URLs),
  `nosniff`; uploaded SVG icons and attachment images are served sandboxed.
- **Logins** — optional TOTP second factor (`docs/auth.md`), PBKDF2-SHA256 600k, equal cost for unknown logins, per-IP throttle (port stripped), per-login stall
  after 10 recent failures (a stall, not a lockout), at most 4 concurrent hashes; logins are `[a-z0-9._-]{2,32}`;
  a password change ends all sessions.

## Secrets at rest (quest=357)

Quest/step/template secret values are sealed with the same server key as the assistant keys (`v1:` + AES-256-GCM),
so a database dump or a backup alone does not reveal them. Values written before are sealed at the next start.
An export made with `secrets=1` carries the plain values (the archive is the user's to protect) and an import seals
them again. **Keep `QUESTS_SECRET_KEY` (or `data/secret.key`) apart from the backups**: sealed values and saved LLM
keys cannot be opened without it — a lost key means re-entering them, nothing else (an account with 2FA signs in
with a recovery code, then re-enrols; `quests-server 2fa-reset <user>` is the host-side way). `quests-server
secrets-check` says whether the current key opens every sealed value; the server logs a warning at start when it
does not. Full write-up: quests note "Шифрование секретов".

## Grants (quest=360)

The API connects as the owner role and drops to `quests_app`. Migration `c8e0a2b4d6f3` takes from it what is the
host's: it cannot rewrite a password hash or re-activate an account (only `last_login_at`), change a workspace beyond its
name and deletion mark, or touch `alembic_version`. The admin commands (`passwd`, `2fa-reset`, `workspace restore`…) run
as the owner (`db.SkipAppRole`). Test: `TestPGAPIRoleGrants`.

## Quotas (quest=358)

A workspace keeps at most `QUESTS_WORKSPACE_QUOTA_MB` (default 2048) of attachments, all file versions counted;
an upload, a new revision, a staged draft file or an import that would pass it gets `413` with the usage. Settings →
Пространство shows the usage; `quests_workspace_attachment_bytes_max` (Prometheus) is the largest workspace.

## The assistant's LLM key (quest=356)

The text → quests assistant runs on the **account's own provider and key** (Settings → Ассистент): Groq, OpenAI,
OpenRouter or any OpenAI-compatible HTTPS endpoint. The key is stored sealed (AES-256-GCM, `go/internal/platform/secretbox`;
key from `QUESTS_SECRET_KEY` or generated once into `data/secret.key`, mode 0600 — **keep that file with the data
volume; without it saved keys cannot be opened and are simply re-entered**), shown back only as a hint (`…a1b2`),
and changed only from a browser session (not with an API token).

- A user-typed endpoint must be `https` and resolve to public addresses only (no loopback, LAN, link-local/metadata,
  CGNAT); the request goes to the address resolved and checked at that moment (TLS and `Host` keep the real name), so DNS
  cannot be re-pointed inward between the check and the connection — also when the host admin routes LLM traffic
  through a proxy; redirects are not followed, and the provider's reply
  body is not echoed back. Otherwise the field would be a server-side request forgery.
- Accounts without a key get the server's key (`QUESTS_GROQ_API_KEY`) only if the host set `QUESTS_LLM_SHARED=1`;
  everyone else is told to add their own. 20 requests / 10 min per account (all of its tokens together) and 8000 characters per request.

## Re-check after changes

```bash
python3 tools/security_probe.py --base http://127.0.0.1:8765 --owner <user>   # asks for the password
```

Run it against a **scratch** instance: it creates a workspace, an attacker account and a few rows (it removes what it can).

## Outgoing requests of scripts (ADR-0004)

The server runs no user code, but a node that fetches an address is our code with a user's address in it. Everything a script
sends from the server goes through `platform/egress` under the **administrator's** policy (`QUESTS_EGRESS_*` in the
environment of the server, `off` by default) and a per-workspace grant that only the admin CLI writes (`quests-server egress
enable <id>`). The address is judged at the connection, after the name is resolved, so rebinding and redirects meet the same
check; internal ranges are refused unless the administrator names them in `QUESTS_EGRESS_ALLOW_NETS`. Size, time, rate and
concurrency are limited; every attempt is in `egress_log`. As a second line, run the `api` container without a route into
the local network (`deploy/README.md`).

## Known gaps (accepted for now)

- Tables without row-level security (`appuser`, `usersession`, `apitoken`, `workspace*`, `backuplog`) are readable by the
  API role (it must read password hashes to check logins): only an SQL injection could reach them. No injection is
  known; every dynamic identifier is validated. What the API role can no longer *change* is listed under "Grants".
- No per-account storage quota for attachments; two-factor sign-in is optional (a policy that requires it for workspace owners is not built); Grafana has its own login only.
- `X-Forwarded-For` is trusted for throttling (the gateway rewrites it; a client on the LAN could forge it — the
  per-login stall covers that).
