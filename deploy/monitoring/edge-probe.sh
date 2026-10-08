#!/bin/bash
# Watchdog for the gateway VPS (quest=345): the one place that still sees the stack when the home
# server itself is down, because it runs outside it. It only LOGS — no alerts (the VPS is a 512 MB
# box that talks to nothing but the tailnet).
#
# Every run (cron, once a minute) fetches the public URL and appends "<epoch>,<up|down>,<ms>" to
# $LOG_DIR/uptime.csv; the first run after a change also writes a line to $LOG_DIR/events.log.
# `edge-report.sh` turns the CSV into availability numbers.
#
#   * * * * *  PROBE_URL=https://quests.84-38-180-123.sslip.io/api/ping /opt/quests-probe/edge-probe.sh
set -uo pipefail
export LC_ALL=C   # dot decimals from curl, no locale surprises in awk
URL="${PROBE_URL:?set PROBE_URL, e.g. https://quests.example.org/api/ping}"
LOG_DIR="${LOG_DIR:-/var/lib/quests-probe}"
KEEP_DAYS="${KEEP_DAYS:-400}"
mkdir -p "$LOG_DIR"
CSV="$LOG_DIR/uptime.csv"; STATE="$LOG_DIR/state"; EVENTS="$LOG_DIR/events.log"

now=$(date +%s)
out=$(curl -fsS -m 10 -o /dev/null -w '%{time_total}' "$URL" 2>/dev/null) && status=up || status=down
ms=$(awk -v s="${out:-0}" 'BEGIN{printf "%d", s*1000}')
echo "$now,$status,$ms" >> "$CSV"

prev=$(cut -d' ' -f1 "$STATE" 2>/dev/null || echo up)
since=$(cut -d' ' -f2 "$STATE" 2>/dev/null || echo "$now")
if [ "$status" != "$prev" ]; then
  if [ "$status" = up ]; then
    echo "$(date -Is -d @"$now") UP   after $((now - since))s down" >> "$EVENTS"
  else
    echo "$(date -Is -d @"$now") DOWN $URL" >> "$EVENTS"
  fi
  echo "$status $now" > "$STATE"
elif [ ! -f "$STATE" ]; then
  echo "$status $now" > "$STATE"
fi

# Trim the CSV once a day (at 03:00) to KEEP_DAYS.
if [ "$(date +%H%M)" = "0300" ]; then
  cutoff=$((now - KEEP_DAYS * 86400))
  awk -F, -v c="$cutoff" '$1 >= c' "$CSV" > "$CSV.tmp" && mv "$CSV.tmp" "$CSV"
fi
