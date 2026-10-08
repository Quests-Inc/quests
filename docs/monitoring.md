# Monitoring (quest=345)

Admin-only and measured from outside the app: a service cannot record its own downtime, and users
already see their runners / HUD / bot in Settings → Сервисы.

| What | Where | Gives |
|---|---|---|
| **Uptime Kuma** | the home server (`arch`), `docker-compose.monitoring.yml` | probes, uptime graph and %, alerts |
| **Edge watchdog** | the gateway VPS, `deploy/monitoring/edge-probe.sh` | availability seen from outside the home server, **log only** (covers the case where `arch` itself is down — Kuma would be down with it) |
| **Prometheus + Grafana** | the home server, same overlay | trends: API latency/errors, quests and runner queue, DB pool and size, host CPU/RAM/disk, backup age |

## Uptime Kuma

1. In the server's `.env`: `MONITORING=1` (and optionally `KUMA_PORT`, default 3001). `ci-deploy.sh` then includes the
   overlay on every deploy, or by hand:
   `docker compose --env-file .env -f deploy/docker/docker-compose.yml -f deploy/docker/docker-compose.monitoring.yml up -d kuma`
2. Open `http://<server>:3001` and create the admin account (done by hand on purpose).
3. Add the probes (needs `pip install uptime-kuma-api`; the password comes from the environment):
   ```bash
   KUMA_PASSWORD=… python3 deploy/monitoring/kuma-monitors.py --url http://<server>:3001 --user <admin> \
       --public https://quests.84-38-180-123.sslip.io
   ```
   It creates API, nginx, Postgres, ClamAV (+ the public address through the gateway, + WebDAV with `--webdav`);
   re-running updates instead of duplicating.
4. Alerts: Kuma → Settings → Notifications → add Telegram (bot token + chat id) and tick "Default enabled" so every probe uses it.
   Kuma in a plain container has no Telegram proxy; if `api.telegram.org` is blocked from the host, use a channel that
   works there (e-mail, a webhook, …).

Kuma's data lives in the `kuma-data` volume; it is not part of the DB backups.

## Metrics: Prometheus + Grafana

Same overlay (`MONITORING=1`). Extra `.env` values: `QUESTS_METRICS_TOKEN` (bearer token for `GET /metrics`; empty switches
the endpoint off), `GRAFANA_ADMIN_PASSWORD`; optional `GRAFANA_PORT` (3002), `PROMETHEUS_RETENTION` (90d).

- `GET /metrics` on the API (token-guarded, aggregates only — no workspace names or ids, sums over all workspaces):
  request count / latency by route pattern, in-flight requests, WebSocket clients, quests by status, runner jobs by state,
  attachments, newest backup age/size, health probes, DB pool, Go runtime. The gateway must not publish it
  (`respond /metrics 404` in the edge Caddyfile).
- Prometheus scrapes `api:8765/metrics`, `node-exporter` (host: CPU, RAM, disk), `postgres-exporter` (size, connections)
  and itself every 15 s; no ports are published for any of them.
- Grafana (`http://<server>:3002`, or behind the gateway at `https://grafana.<base domain>` with `GRAFANA_ROOT_URL` in `.env`; needs port 3002 in the Tailscale ACL grant, user `admin`, password from `.env`) comes with the Prometheus data source and the
  «Quests — обзор» dashboard provisioned from `deploy/monitoring/grafana/`. Edit the JSON in the repo, not in the UI
  (provisioned dashboards are read-only there).

## Edge watchdog (gateway VPS)

`edge-probe.sh` fetches the public URL once a minute and appends `epoch,up|down,ms` to `/var/lib/quests-probe/uptime.csv`,
plus a line to `events.log` whenever the state flips. No alerts: the 512 MB box only logs.

```bash
# on the VPS (root): files in /opt/quests-probe/, cron every minute
* * * * *  PROBE_URL=https://quests.84-38-180-123.sslip.io/api/ping /opt/quests-probe/edge-probe.sh
ssh root@84.38.180.123 /opt/quests-probe/edge-report.sh     # 24h / 7d / 30d / all-time %, outages, last events
```
