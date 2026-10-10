#!/usr/bin/env bash
# Start (or update) doco-cd on this host. This is the one step that needs a hand on the server: it runs from
# .github/workflows/bootstrap-deploy.yml on a self-hosted runner for as long as nobody has shell access,
# or directly by an admin. It reads everything from the environment, writes it into $DEPLOY_STATE with
# owner-only permissions, and brings the doco-cd stack up. Safe to run again (it rewrites the same files).
#
# Required env:
#   DEPLOY_REPO_URL     https URL of the repository doco-cd polls
#   DEPLOY_GIT_TOKEN    a read-only token for that repository
#   SOPS_AGE_KEY        the age private key (the whole "AGE-SECRET-KEY-…" line)
#   GHCR_USER, GHCR_TOKEN  a login that may pull the private images (read:packages)
# Optional env:
#   DEPLOY_STATE        where state lives (default /srv/quests-deploy)
#   DEPLOY_REF          branch to follow (default prod)
#   TG_BOT_TOKEN, TG_CHAT_ID   enable Telegram notifications about deployments (both, or neither)
#   DOCO_CD_VERSION     image tag of doco-cd (default latest; pin it once you have seen it work)
set -euo pipefail
umask 077

: "${DEPLOY_REPO_URL:?}" "${DEPLOY_GIT_TOKEN:?}" "${SOPS_AGE_KEY:?}" "${GHCR_USER:?}" "${GHCR_TOKEN:?}"
state="${DEPLOY_STATE:-/srv/quests-deploy}"
here="$(cd "$(dirname "$0")" && pwd)"

if ! [[ "$SOPS_AGE_KEY" =~ ^AGE-SECRET-KEY-[0-9A-Z]+$ ]]; then
  echo "bootstrap: SOPS_AGE_KEY is not an age private key (one AGE-SECRET-KEY-… line)" >&2
  exit 2
fi

mkdir -p "$state/secrets" "$state/docker-config"
printf '%s' "$DEPLOY_GIT_TOKEN" >"$state/secrets/git_access_token"
printf '%s\n' "$SOPS_AGE_KEY" >"$state/secrets/sops_age_key"
auth="$(printf '%s:%s' "$GHCR_USER" "$GHCR_TOKEN" | base64 | tr -d '\n')"
printf '{"auths":{"ghcr.io":{"auth":"%s"}}}\n' "$auth" >"$state/docker-config/config.json"
chmod 600 "$state"/secrets/* "$state/docker-config/config.json"

{
  echo "DEPLOY_STATE=$state"
  echo "DEPLOY_REPO_URL=$DEPLOY_REPO_URL"
  echo "DEPLOY_REF=${DEPLOY_REF:-prod}"
  if [ -n "${DOCO_CD_VERSION:-}" ]; then echo "DOCO_CD_VERSION=$DOCO_CD_VERSION"; fi
  if [ -n "${TG_BOT_TOKEN:-}" ] && [ -n "${TG_CHAT_ID:-}" ]; then echo "COMPOSE_PROFILES=notify"; fi
} >"$state/.env"

# Notifications: only with a bot token and a chat; otherwise doco-cd runs silent.
rm -f "$state/notify.env"
if [ -n "${TG_BOT_TOKEN:-}" ] && [ -n "${TG_CHAT_ID:-}" ]; then
  {
    echo "APPRISE_API_URL=http://apprise:8000/notify"
    echo "APPRISE_NOTIFY_URLS=tgram://${TG_BOT_TOKEN}/${TG_CHAT_ID}"
    echo "APPRISE_NOTIFY_LEVEL=success"
  } >"$state/notify.env"
fi

docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" pull
docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" up -d --remove-orphans
docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" ps
echo "bootstrap: doco-cd follows ${DEPLOY_REF:-prod} of $DEPLOY_REPO_URL"
