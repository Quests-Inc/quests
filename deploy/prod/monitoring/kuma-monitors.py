#!/usr/bin/env python3
"""Create (or update) the Quests probes in Uptime Kuma. Idempotent: monitors are matched by name.

    pip install uptime-kuma-api          # one-off, anywhere that can reach Kuma
    KUMA_PASSWORD=… python3 deploy/prod/monitoring/kuma-monitors.py \\
        --url http://192.168.1.11:3001 --user admin [--public https://quests.example.org] [--webdav http://…]

The admin account is created by hand in the Kuma UI on first start; the password is read
from the environment so it stays out of shell history. Alert channels (Telegram etc.) hold
secrets and are set up in the UI: Settings → Notifications, "default enabled" to attach them
to every probe.
"""

from __future__ import annotations

import argparse
import os
import sys

from uptime_kuma_api import MonitorType, UptimeKumaApi


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--url", required=True, help="Kuma base URL")
    ap.add_argument("--user", required=True)
    ap.add_argument("--public", help="public URL of the stack (through the gateway), probes /api/ping")
    ap.add_argument("--webdav", help="WebDAV base URL to probe")
    ap.add_argument("--interval", type=int, default=60)
    args = ap.parse_args()
    password = os.environ.get("KUMA_PASSWORD")
    if not password:
        print("set KUMA_PASSWORD", file=sys.stderr)
        return 2

    common = {"interval": args.interval, "retryInterval": 20, "maxretries": 2}
    wanted = [
        {"name": "API", "type": MonitorType.HTTP, "url": "http://api:8765/api/ping", **common},
        {"name": "Фронт (nginx)", "type": MonitorType.HTTP, "url": "http://frontend/", **common},
        {"name": "Postgres", "type": MonitorType.PORT, "hostname": "postgres", "port": 5432, **common},
        {"name": "ClamAV", "type": MonitorType.PORT, "hostname": "clamav", "port": 3310, **common},
    ]
    if args.public:
        wanted.append({"name": "Публичный адрес (через шлюз)", "type": MonitorType.HTTP,
                       "url": args.public.rstrip("/") + "/api/ping", **common})
    if args.webdav:
        wanted.append({"name": "WebDAV", "type": MonitorType.HTTP, "url": args.webdav,
                       "accepted_statuscodes": ["200-299", "401"], **common})

    api = UptimeKumaApi(args.url)
    try:
        api.login(args.user, password)
        existing = {m["name"]: m for m in api.get_monitors()}
        for spec in wanted:
            cur = existing.get(spec["name"])
            if cur:
                api.edit_monitor(cur["id"], **spec)
                print(f"updated  {spec['name']}")
            else:
                api.add_monitor(**spec)
                print(f"created  {spec['name']}")
    finally:
        api.disconnect()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
