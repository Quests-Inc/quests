#!/bin/bash
# Daily backup of the monitoring volumes (quest=345): Uptime Kuma (probes, history, accounts, 2FA)
# and Grafana (users, preferences). Prometheus is left out on purpose: its 90-day history is large
# and rebuilds itself; the dashboard and data source come from the repo.
#
# SQLite inside the volumes is only consistent when nothing writes to it, so each service is stopped
# for the few seconds the tar takes (Kuma shows a one-minute gap in its own history).
#
# Cron (user crontab on the server):
#   30 4 * * *  $HOME/Documents/projects/quests/deploy/docker/backup-monitoring.sh >> $HOME/backup-monitoring.log 2>&1
#
# Restore (service stopped):
#   docker stop quests-kuma
#   docker run --rm -v quests_kuma-data:/data -v "$PWD":/b alpine:3.20 sh -c 'rm -rf /data/* && tar -xzf /b/kuma-<ts>.tar.gz -C /data'
#   docker start quests-kuma
set -uo pipefail
OUT="${BACKUP_DIR:-$HOME/QuestsBackups-monitoring}"
KEEP="${KEEP:-14}"
PROJECT="${COMPOSE_PROJECT:-quests}"
IMAGE="${TAR_IMAGE:-alpine:3.20}"
# Second copy on another machine, best effort (the PC is not always on).
REMOTE="${BACKUP_REMOTE:-amarant@192.168.1.15}"
REMOTE_DIR="${BACKUP_REMOTE_DIR:-QuestsBackups/monitoring}"

mkdir -p "$OUT"
ts=$(date +%Y%m%d-%H%M%S)
status=0

backup_one() {
  local svc=$1 vol="${PROJECT}_$2" ctr="quests-$1" file="$OUT/$1-$ts.tar.gz" was_running
  was_running=$(docker inspect -f '{{.State.Running}}' "$ctr" 2>/dev/null || echo false)
  [ "$was_running" = true ] && docker stop -t 20 "$ctr" >/dev/null
  if docker run --rm -v "$vol":/data:ro "$IMAGE" tar -C /data -czf - . > "$file.tmp" && [ -s "$file.tmp" ]; then
    mv "$file.tmp" "$file"
    echo "$(date -Is) $svc: $(du -h "$file" | cut -f1)"
  else
    rm -f "$file.tmp"
    echo "$(date -Is) $svc: BACKUP FAILED"
    status=1
  fi
  [ "$was_running" = true ] && docker start "$ctr" >/dev/null
  # keep the newest $KEEP per service
  ls -1t "$OUT/$1"-*.tar.gz 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
}

backup_one kuma kuma-data
backup_one grafana grafana-data

if ssh -o BatchMode=yes -o ConnectTimeout=5 "$REMOTE" "mkdir -p '$REMOTE_DIR'" 2>/dev/null; then
  if command -v rsync >/dev/null; then
    rsync -a --delete -e "ssh -o BatchMode=yes" "$OUT"/ "$REMOTE:$REMOTE_DIR/" && echo "$(date -Is) copied to $REMOTE:$REMOTE_DIR"
  else
    scp -q -o BatchMode=yes "$OUT"/*"$ts"* "$REMOTE:$REMOTE_DIR/" && echo "$(date -Is) copied to $REMOTE:$REMOTE_DIR"
  fi
else
  echo "$(date -Is) remote $REMOTE unreachable: local copy only"
fi
exit $status
