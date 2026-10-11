# Monitoring (quest=345)

Admin-only and measured from outside the app: a service cannot record its own downtime, and users
already see their runners / HUD / bot in Settings → Сервисы.

| What | Where | Gives |
|---|---|---|
| **Prometheus + Grafana** | the home server (`arch`), `docker-compose.monitoring.yml` (deploy repository) | probes and their uptime / incident history, trends: API latency/errors, quests and runner queue, DB pool and size, host CPU/RAM/disk, backup age |
| **Edge watchdog** | the gateway VPS, `monitoring/edge-probe.sh` (deploy repository) | availability seen from outside the home server, **log only** (covers the case where `arch` itself is down — Prometheus and Grafana would be down with it) |

## Probes and availability

The stack ships with every release: `docker-compose.monitoring.yml` of the deploy repository (`Quests-Inc/quests-deploy`) is listed in
its `.doco-cd.yml` (drop it from `compose_files` to switch monitoring off). Uptime Kuma used to do the probing; it was replaced by
Prometheus (blackbox exporter) and Grafana, which already had the metrics.

- **HTTP probes** (job `blackbox`, module `http_2xx`): `api` (`api:8765/api/ping`), `nginx` (`frontend/api/ping`, the front door) and `public`
  (the public address through the gateway). The last one also gives the gateway certificate expiry (`probe_ssl_earliest_cert_expiry`).
- **TCP probes** (job `blackbox_tcp`, module `tcp_connect`): `clamav` (`clamav:3310`).
- Postgres comes from `postgres-exporter` (`pg_up`, size, connections).
- The «Quests — обзор» dashboard shows them in the «Доступность» section: current state, 30-day uptime % per probe, a state timeline
  (red = down: that is the incident list) and the days left on the gateway certificate.
- Alerts are **not configured** (a deliberate choice). If they are wanted: Grafana → Alerting → a contact point (Telegram, e-mail, a webhook)
  and a rule on `probe_success == 0`. `api.telegram.org` may need a proxy from the host.

`deploy/docker/backup-monitoring.sh` (cron, daily 04:30) tars the Grafana volume (14 kept) into `~/QuestsBackups-monitoring/` and mirrors
it to the PC (`~/QuestsBackups/monitoring/`) when it is on. Grafana is stopped for the seconds the tar takes so its SQLite file is
consistent. Prometheus is not backed up (large, rebuilds itself; dashboards and data source come from the repo). Restore steps are in the
script header.

## Metrics: Prometheus + Grafana

Same stack. Extra values in `secrets.enc.env` / `config.env`: `QUESTS_METRICS_TOKEN` (bearer token for `GET /metrics`; empty switches
the endpoint off), `GRAFANA_ADMIN_PASSWORD`; optional `GRAFANA_PORT` (3002), `PROMETHEUS_RETENTION` (90d).

- `GET /metrics` on the API (token-guarded, aggregates only — no workspace names or ids, sums over all workspaces):
  request count / latency by route pattern, in-flight requests, WebSocket clients, quests by status, runner jobs by state,
  attachments, newest backup age/size, health probes, DB pool, Go runtime. The gateway must not publish it
  (`respond /metrics 404` in the edge Caddyfile).
- Prometheus scrapes `api:8765/metrics`, `node-exporter` (host: CPU, RAM, disk), `postgres-exporter` (size, connections)
  and itself every 15 s; no ports are published for any of them.
- Grafana (`http://<server>:3002`, or behind the gateway at `https://grafana.<base domain>` with `GRAFANA_ROOT_URL` in `.env`; needs port 3002 in the Tailscale ACL grant, user `admin`, password from `.env`) comes with the Prometheus data source and the
  «Quests — обзор» dashboard provisioned from `monitoring/grafana/` of the deploy repository. Edit the JSON in the repo, not in the UI
  (provisioned dashboards are read-only there).

## Edge watchdog (gateway VPS)

`edge-probe.sh` fetches the public URL once a minute and appends `epoch,up|down,ms` to `/var/lib/quests-probe/uptime.csv`,
plus a line to `events.log` whenever the state flips. No alerts: the 512 MB box only logs.

```bash
# on the VPS (root): files in /opt/quests-probe/, cron every minute
* * * * *  PROBE_URL=https://quests.84-38-180-123.sslip.io/api/ping /opt/quests-probe/edge-probe.sh
ssh root@84.38.180.123 /opt/quests-probe/edge-report.sh     # 24h / 7d / 30d / all-time %, outages, last events
```
