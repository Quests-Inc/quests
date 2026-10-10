#!/usr/bin/env bash
# Start (or update) doco-cd on this host. This is the one step that needs a hand on the server: it runs from
# .github/workflows/bootstrap-deploy.yml on a self-hosted runner for as long as nobody has shell access,
# or directly by an admin. It reads everything from the environment, writes it into $DEPLOY_STATE with
# owner-only permissions, and brings the doco-cd stack up. Safe to run again (it rewrites the same files).
#
# Required env:
#   DEPLOY_REPO_URL     https URL of the repository doco-cd polls
#   DEPLOY_GIT_TOKEN    a read-only token for that repository
#   GHCR_USER, GHCR_TOKEN  a login that may pull the private images (read:packages)
# Secrets for the stack, one or both (SOPS wins where both define a variable):
#   SOPS_AGE_KEY        the age private key (the whole "AGE-SECRET-KEY-…" line) that opens secrets.enc.env
#   LEGACY_ENV_FILE     path on THIS host of the old .env to keep using (default: none)
#   LEGACY_ENV          the content of such a .env (takes precedence over LEGACY_ENV_FILE)
# Optional env:
#   DEPLOY_STATE        where state lives (default /srv/quests-deploy)
#   DEPLOY_REF          branch to follow (default prod)
#   TG_BOT_TOKEN, TG_CHAT_ID   enable Telegram notifications about deployments (both, or neither)
#   DOCO_CD_VERSION     image tag of doco-cd (default latest; pin it once you have seen it work)
set -euo pipefail
umask 077

: "${DEPLOY_REPO_URL:?}" "${DEPLOY_GIT_TOKEN:?}" "${GHCR_USER:?}" "${GHCR_TOKEN:?}"
state="${DEPLOY_STATE:-/srv/quests-deploy}"
here="$(cd "$(dirname "$0")" && pwd)"

if [ -n "${SOPS_AGE_KEY:-}" ] && ! [[ "$SOPS_AGE_KEY" =~ ^AGE-SECRET-KEY-[0-9A-Z]+$ ]]; then
  echo "bootstrap: SOPS_AGE_KEY is not an age private key (one AGE-SECRET-KEY-… line)" >&2
  exit 2
fi

mkdir -p "$state/secrets" "$state/docker-config"
printf '%s' "$DEPLOY_GIT_TOKEN" >"$state/secrets/git_access_token"
auth="$(printf '%s:%s' "$GHCR_USER" "$GHCR_TOKEN" | base64 | tr -d '\n')"
printf '{"auths":{"ghcr.io":{"auth":"%s"}}}\n' "$auth" >"$state/docker-config/config.json"

# doco-cd's own optional settings: the age key (when there is one) and notifications (when there is a bot).
rm -f "$state/doco.env" "$state/secrets/sops_age_key"
if [ -n "${SOPS_AGE_KEY:-}" ]; then
  printf '%s\n' "$SOPS_AGE_KEY" >"$state/secrets/sops_age_key"
  echo "SOPS_AGE_KEY_FILE=/secrets/sops_age_key" >>"$state/doco.env"
fi
if [ -n "${TG_BOT_TOKEN:-}" ] && [ -n "${TG_CHAT_ID:-}" ]; then
  {
    echo "APPRISE_API_URL=http://apprise:8000/notify"
    echo "APPRISE_NOTIFY_URLS=tgram://${TG_BOT_TOKEN}/${TG_CHAT_ID}"
    echo "APPRISE_NOTIFY_LEVEL=success"
  } >>"$state/doco.env"
fi

# The old .env: from the content given, else from a file on this host, else an empty file (the mount must exist).
if [ -n "${LEGACY_ENV:-}" ]; then
  printf '%s\n' "$LEGACY_ENV" >"$state/legacy.env"
elif [ -n "${LEGACY_ENV_FILE:-}" ] && [ -f "$LEGACY_ENV_FILE" ]; then
  cp "$LEGACY_ENV_FILE" "$state/legacy.env"
else
  [ -n "${LEGACY_ENV_FILE:-}" ] && echo "bootstrap: LEGACY_ENV_FILE=$LEGACY_ENV_FILE does not exist: continuing without a legacy .env" >&2
  : >"$state/legacy.env"
fi
chmod 600 "$state"/secrets/* "$state/docker-config/config.json" "$state/legacy.env"
[ ! -f "$state/doco.env" ] || chmod 600 "$state/doco.env"

{
  echo "DEPLOY_STATE=$state"
  echo "DEPLOY_REPO_URL=$DEPLOY_REPO_URL"
  echo "DEPLOY_REF=${DEPLOY_REF:-prod}"
  if [ -n "${DOCO_CD_VERSION:-}" ]; then echo "DOCO_CD_VERSION=$DOCO_CD_VERSION"; fi
  if [ -n "${TG_BOT_TOKEN:-}" ] && [ -n "${TG_CHAT_ID:-}" ]; then echo "COMPOSE_PROFILES=notify"; fi
} >"$state/.env"

docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" pull
docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" up -d --remove-orphans
docker compose --env-file "$state/.env" -f "$here/docker-compose.yml" ps
echo "bootstrap: doco-cd follows ${DEPLOY_REF:-prod} of $DEPLOY_REPO_URL"
