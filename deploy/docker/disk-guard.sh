#!/bin/bash
# Weekly housekeeping for the prod host (quest=346). Every `:main` image pull leaves the
# previous image behind as an untagged ("dangling") layer set; unchecked they filled the disk
# to 100% and made the CI deploy fail mid-pull. Only dangling images and build cache are
# removed — never tagged images (other projects on this host build theirs locally).
#
# Cron (user crontab on the server):
#   0 4 * * 0  $HOME/Documents/projects/quests/deploy/docker/disk-guard.sh >> $HOME/disk-guard.log 2>&1
#
# If the disk is still at or above THRESHOLD afterwards, a quest is created through the
# local API (needs QUESTS_API_TOKEN in .env) so it shows up on the HUD.
set -uo pipefail
THRESHOLD="${DISK_GUARD_THRESHOLD:-85}"
REPO_DIR="${REPO_DIR:-$HOME/Documents/projects/quests}"
MOUNT="${DISK_GUARD_MOUNT:-/}"

used() { df --output=pcent "$MOUNT" | tail -1 | tr -dc '0-9'; }

before=$(used)
docker image prune -f >/dev/null 2>&1
docker builder prune -f >/dev/null 2>&1
after=$(used)
echo "$(date -Is) disk ${before}% -> ${after}%"

if [ "$after" -ge "$THRESHOLD" ]; then
  token=$(grep -m1 '^QUESTS_API_TOKEN=' "$REPO_DIR/.env" 2>/dev/null | cut -d= -f2-)
  if [ -n "$token" ]; then
    curl -fsS -m 10 -X POST http://127.0.0.1:8765/api/quests \
      -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
      -d "{\"title\":\"Диск прода заполнен на ${after}%\",\"description\":\"После очистки dangling-образов всё ещё ${after}% (порог ${THRESHOLD}%). Смотри docker system df и volumes.\",\"significance\":\"rare\",\"status\":\"active\"}" \
      >/dev/null && echo "alert quest created"
  else
    echo "disk at ${after}% but no QUESTS_API_TOKEN to raise a quest"
  fi
fi
