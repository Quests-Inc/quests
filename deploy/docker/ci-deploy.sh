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

# shellcheck disable=SC2086
$COMPOSE pull
# shellcheck disable=SC2086
$COMPOSE up -d --remove-orphans

$COMPOSE ps
