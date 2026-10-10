#!/usr/bin/env bash
# Pull the prod database (+ app files such as icons) into the local dev PostgreSQL and ./data.
#
#   ./scripts/pull-prod-data.sh
#   ./scripts/pull-prod-data.sh --start          # then ./scripts/dev-api-docker.sh
#   ./scripts/pull-prod-data.sh --pull-image     # also refresh quests-api:local from GHCR
#
# Needs: ssh to the prod host, docker on both sides, the local dev database up (`just dev-db`).
# The dump is taken inside the prod postgres container; it REPLACES the local dev database's contents.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="${QUESTS_DATA_DIR:-$ROOT/data}"
SSH_HOST="${QUESTS_PROD_SSH:?set QUESTS_PROD_SSH, e.g. user@host}"
API_CONTAINER="${QUESTS_PROD_CONTAINER:-quests-api}"
PG_CONTAINER="${QUESTS_PROD_PG_CONTAINER:-quests-postgres}"
LOCAL_PG="${QUESTS_LOCAL_PG_CONTAINER:-quests-pg-dev}"
IMAGE="${QUESTS_API_IMAGE:-ghcr.io/valirum/quests-api:main}"

START=0
PULL_IMAGE=0
for arg in "$@"; do
  case "$arg" in
    --start) START=1 ;;
    --pull-image) PULL_IMAGE=1 ;;
    -h|--help)
      cat <<DOC
usage: $0 [--pull-image] [--start]

  Dump the database of \`$PG_CONTAINER\` on \`$SSH_HOST\` into the local container \`$LOCAL_PG\`
  (replacing its data) and copy /app/data of \`$API_CONTAINER\` into \`$DATA\`.

env:
  QUESTS_PROD_SSH           required, e.g. user@host
  QUESTS_PROD_CONTAINER     api container on prod, default: quests-api
  QUESTS_PROD_PG_CONTAINER  postgres container on prod, default: quests-postgres
  QUESTS_LOCAL_PG_CONTAINER local dev postgres, default: quests-pg-dev
  QUESTS_DATA_DIR           default: <repo>/data
  QUESTS_API_IMAGE          default: ghcr.io/valirum/quests-api:main
DOC
      exit 0
      ;;
    *)
      echo "unknown arg: $arg (try --help)" >&2
      exit 2
      ;;
  esac
done

docker inspect "$LOCAL_PG" >/dev/null 2>&1 || { echo "local database container $LOCAL_PG is not running: just dev-db" >&2; exit 1; }
mkdir -p "$DATA"

echo "dumping $SSH_HOST:$PG_CONTAINER → $LOCAL_PG" >&2
docker exec "$LOCAL_PG" psql -U quests -d postgres -qc "DROP DATABASE IF EXISTS quests WITH (FORCE)" -c "CREATE DATABASE quests"
ssh -o BatchMode=yes "$SSH_HOST" "docker exec $PG_CONTAINER pg_dump -U quests --format=custom --no-owner quests" \
  | docker exec -i "$LOCAL_PG" pg_restore -U quests -d quests --no-owner

notes="$(docker exec "$LOCAL_PG" psql -U quests -d quests -Atc 'SELECT COUNT(*) FROM note' 2>/dev/null || echo '?')"
quests="$(docker exec "$LOCAL_PG" psql -U quests -d quests -Atc 'SELECT COUNT(*) FROM quest' 2>/dev/null || echo '?')"
echo "ok: database restored (notes=$notes quests=$quests)" >&2

# Keep workstation overlay HUD config if present.
overlay_bak=""
if [[ -f "$DATA/overlay.json" ]]; then
  overlay_bak="$(mktemp)"
  cp -a "$DATA/overlay.json" "$overlay_bak"
fi
echo "pulling $SSH_HOST:$API_CONTAINER:/app/data → $DATA" >&2
ssh -o BatchMode=yes "$SSH_HOST" "docker exec $API_CONTAINER tar czf - -C /app/data ." | tar xzf - -C "$DATA"
if [[ -n "$overlay_bak" ]]; then
  mv -f "$overlay_bak" "$DATA/overlay.json"
fi

if [[ "$PULL_IMAGE" -eq 1 ]]; then
  echo "pulling $IMAGE → quests-api:local" >&2
  if docker pull "$IMAGE"; then
    docker tag "$IMAGE" quests-api:local
  else
    echo "ghcr pull failed — docker save|load from prod" >&2
    ssh -o BatchMode=yes "$SSH_HOST" "docker save $IMAGE" | docker load
    docker tag "$IMAGE" quests-api:local
  fi
fi

if [[ "$START" -eq 1 ]]; then
  exec "$ROOT/scripts/dev-api-docker.sh"
fi

echo "next: ./scripts/dev-api-docker.sh   # UI http://127.0.0.1:8765/" >&2
