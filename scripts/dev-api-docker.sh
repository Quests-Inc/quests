#!/usr/bin/env bash
# Local Quests API in Docker (quests-api-dev on 127.0.0.1:8765).
# SPA: rebuild with `just build-frontend` — dist is bind-mounted.
# Go/Python image: pass --rebuild.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/docker/docker-compose.dev.yml")
REBUILD=0
for arg in "$@"; do
  case "$arg" in
    --rebuild|-b) REBUILD=1 ;;
    -h|--help)
      echo "usage: $0 [--rebuild]"
      exit 0
      ;;
  esac
done

# Free :8765 from a host binary / old container.
if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -t -iTCP:8765 -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "${pids}" ]]; then
    echo "stopping host listeners on :8765: ${pids}" >&2
    # shellcheck disable=SC2086
    kill ${pids} 2>/dev/null || true
    sleep 0.4
  fi
fi
docker rm -f quests-api-dev >/dev/null 2>&1 || true

if [[ ! -d "$ROOT/frontend/dist" ]]; then
  echo "frontend/dist missing — building SPA…" >&2
  (cd "$ROOT/frontend" && npm run build)
fi

cd "$ROOT"
if [[ "$REBUILD" -eq 1 ]]; then
  "${COMPOSE[@]}" up -d --build --force-recreate api
else
  # Use existing image if present; build once otherwise.
  if ! docker image inspect quests-api:local >/dev/null 2>&1; then
    "${COMPOSE[@]}" build api
  fi
  "${COMPOSE[@]}" up -d --force-recreate api
fi

echo -n "waiting for /api/ping" >&2
for _ in $(seq 1 40); do
  if curl -fsS --noproxy '*' http://127.0.0.1:8765/api/ping >/dev/null 2>&1; then
    echo " ok" >&2
    curl -fsS --noproxy '*' http://127.0.0.1:8765/api/health | head -c 120
    echo
    exit 0
  fi
  echo -n . >&2
  sleep 0.5
done
echo " timeout" >&2
"${COMPOSE[@]}" logs --tail 80 api >&2 || true
exit 1
