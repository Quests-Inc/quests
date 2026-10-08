#!/bin/bash
# Availability numbers from edge-probe.sh's CSV: 24 h / 7 d / 30 d / all time, current state, outages.
#   ssh root@<vps> /opt/quests-probe/edge-report.sh
set -uo pipefail
export LC_ALL=C   # dot decimals from curl, no locale surprises in awk
LOG_DIR="${LOG_DIR:-/var/lib/quests-probe}"
CSV="$LOG_DIR/uptime.csv"
[ -s "$CSV" ] || { echo "no data yet in $CSV"; exit 0; }
now=$(date +%s)
for span in "24h:86400" "7d:604800" "30d:2592000" "all:0"; do
  label=${span%%:*}; secs=${span##*:}
  awk -F, -v now="$now" -v secs="$secs" -v label="$label" '
    secs == 0 || $1 >= now - secs { n++; if ($2 == "up") { u++; ms += $3 } }
    END { if (n) printf "%-4s %6.2f%%  (%d probes, avg %d ms when up)\n", label, 100*u/n, n, (u ? ms/u : 0); else printf "%-4s no data\n", label }' "$CSV"
done
echo "now: $(tail -1 "$CSV" | awk -F, '{print $2}')   last probe: $(date -Is -d @"$(tail -1 "$CSV" | cut -d, -f1)")"
echo "--- outages (consecutive failed probes, last 30 days)"
# mawk (Debian's default awk) has no strftime: awk prints epochs, date formats them.
awk -F, -v now="$now" '
  $1 < now - 2592000 { next }
  $2 == "down" { if (!d) { start = $1; d = 1 } last = $1; next }
  d { printf "%d %d\n", start, (last - start) / 60 + 1; d = 0 }
  END { if (d) printf "%d %d ongoing\n", start, (last - start) / 60 + 1 }' "$CSV" | tail -20 |
  while read -r start mins tail; do
    echo "$(date -d @"$start" '+%Y-%m-%d %H:%M')  ${mins} min ${tail:-}"
  done
if [ -f "$LOG_DIR/events.log" ]; then echo "--- last events"; tail -5 "$LOG_DIR/events.log"; fi
