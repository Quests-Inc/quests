#!/bin/bash
# Runs ON THE SERVER, invoked over SSH by the deploy job in
# .github/workflows/main.yml — replaces Watchtower's poll-and-swap with an
# explicit step at the end of a green CI run.
#
# The SSH key CI uses for this is restricted in authorized_keys to only ever
# run this exact command (see docs/deploy-ssh.md) — so even a leaked key
# can't do anything beyond "redeploy from origin/main", not arbitrary shell.
set -euo pipefail

REPO_DIR="$HOME/Documents/projects/quests"
COMPOSE="docker compose --env-file .env -f deploy/docker/docker-compose.yml"

cd "$REPO_DIR"
# PostgreSQL overlay is switched on by POSTGRES_PASSWORD in .env (docs/postgres.md).
if grep -q '^POSTGRES_PASSWORD=.' .env 2>/dev/null; then
  COMPOSE="$COMPOSE -f deploy/docker/docker-compose.pg.yml"
fi
git fetch origin main
git reset --hard origin/main

# Uptime Kuma (docs/monitoring.md) is switched on by MONITORING=1 in .env.
if grep -q '^MONITORING=1' .env 2>/dev/null; then
  COMPOSE="$COMPOSE -f deploy/docker/docker-compose.monitoring.yml"
fi

# Our own images (built by this CI run) must arrive: retry a flaky registry a few times.
# Third-party images (postgres, grafana, nginx…) only refresh in passing: a Docker Hub
# timeout on one of them must not stop a deploy whose images are already pulled — the
# running ones stay, and the next deploy tries again.
pull_retry() {
  local n
  for n in 1 2 3; do
    # shellcheck disable=SC2086
    if $COMPOSE pull "$@"; then return 0; fi
    echo "pull failed (attempt $n/3): $*" >&2
    sleep $((n * 10))
  done
  return 1
}
pull_retry api bot stt
# shellcheck disable=SC2086
$COMPOSE pull --ignore-buildable || echo "warning: third-party image refresh failed; keeping the images already present" >&2
# shellcheck disable=SC2086
$COMPOSE up -d --remove-orphans

$COMPOSE ps
