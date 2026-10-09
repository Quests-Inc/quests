# Security model (accounts, workspaces, invited members)

Who can do what, what is enforced where, and how to re-check it.

## Trust boundaries

| Who | Can | Cannot |
|---|---|---|
| Anyone (no account) | `/api/ping`, login (throttled), open an invitation link and sign up through it | anything else |
| Member of a workspace | read and write that workspace's quests, notes, attachments, tags; make API tokens for it; export nothing | set commands, secrets, runners; invite; rename/delete/export; see other workspaces; see server health details |
| Owner of a workspace | everything above + commands, secrets, runners, invitations, rename, delete, export | invite into a *trusted* workspace |
| Operator (owner of a trusted workspace / host admin) | commands that run **on the server** (`sh -c`), server health, heartbeats, `quests-server` admin CLI | — |

A **trusted** workspace is the one place server-side commands may run, so it is the host. It has exactly the owners
you add from the host (`quests-server workspace trust`), and **no invitations**: a member would get the host.

## Where each rule lives

- **Row isolation** — PostgreSQL row-level security (`workspace_id = app.workspace_id`), the API role `quests_app` has no
  BYPASSRLS. A request without a workspace sees nothing. Tests: `go/internal/db` (`QUESTS_TEST_PG_URL=…`).
- **Workspace per request** — `withWorkspace` in `go/internal/httpapi/workspaces.go`: sessions/tokens are pinned, the
  `X-Quests-Workspace` header is checked against membership. The assistant's loopback token must name the workspace
  (it used to fall into the default one — an invited user could rewrite the owner's quests through it).
- **Commands** — `guardAuthor` (only owners write a command; editing around an existing one is fine) and
  `guardExec` (a command without a runner needs a trusted workspace), both in `go/internal/store/workspaces.go`.
  Secrets names that would bend how a process starts (`PATH`, `LD_*`, `BASH_ENV`, `QUESTS_*`…) are refused and never exported.
- **Exports** — PDF is rendered by `go/internal/httpapi/pdfrender/render.py` with every external fetch refused
  (the HTML is user-supplied; WeasyPrint would embed `file://` targets into the PDF).
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
host's: it cannot flip `workspace.trusted` (the switch for running commands on the host), rewrite a password hash or
re-activate an account (only `last_login_at`), or touch `alembic_version`. The admin commands (`quests-server
workspace trust|untrust`, `passwd`, `2fa-reset`…) run as the owner (`db.SkipAppRole`). Test: `TestPGAPIRoleGrants`.

## Quotas (quest=358)

A workspace keeps at most `QUESTS_WORKSPACE_QUOTA_MB` (default 2048) of attachments, all file versions counted;
an upload, a new revision, a staged draft file or an import that would pass it gets `413` with the usage. Settings →
Пространство shows the usage; `quests_workspace_attachment_bytes_max` (Prometheus) is the largest workspace.

## The assistant's LLM key (quest=356)

The text → quests assistant runs on the **account's own provider and key** (Settings → Ассистент): Groq, OpenAI,
OpenRouter or any OpenAI-compatible HTTPS endpoint. The key is stored sealed (AES-256-GCM, `go/internal/secretbox`;
key from `QUESTS_SECRET_KEY` or generated once into `data/secret.key`, mode 0600 — **keep that file with the data
volume; without it saved keys cannot be opened and are simply re-entered**), shown back only as a hint (`…a1b2`),
and changed only from a browser session (not with an API token).

- A user-typed endpoint must be `https` and resolve to public addresses only (no loopback, LAN, link-local/metadata,
  CGNAT); the request goes to the address resolved and checked at that moment (TLS and `Host` keep the real name), so DNS
  cannot be re-pointed inward between the check and the connection — also when the operator routes LLM traffic
  through a proxy; redirects are not followed, and the provider's reply
  body is not echoed back. Otherwise the field would be a server-side request forgery.
- Accounts without a key get the server's key (`QUESTS_GROQ_API_KEY`) only if they are the operator, or the operator set
  `QUESTS_LLM_SHARED=1`; everyone else is told to add their own. 20 requests / 10 min per account (all of its tokens together) and 8000 characters per request.

## Re-check after changes

```bash
python3 scripts/security_probe.py --base http://127.0.0.1:8765 --owner <user>   # asks for the password
```

Run it against a **scratch** instance: it creates a workspace, an attacker account and a few rows (it removes what it can).

## Known gaps (accepted for now)

- Tables without row-level security (`appuser`, `usersession`, `apitoken`, `workspace*`, `backuplog`) are readable by the
  API role (it must read password hashes to check logins): only an SQL injection could reach them. No injection is
  known; every dynamic identifier is validated. What the API role can no longer *change* is listed under "Grants".
- No per-account storage quota for attachments; two-factor sign-in is optional (an operator policy that requires it for owners of a trusted workspace is not built); Grafana/Kuma have their own logins only.
- `X-Forwarded-For` is trusted for throttling (the gateway rewrites it; a client on the LAN could forge it — the
  per-login stall covers that).
