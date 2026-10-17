#!/usr/bin/env bash
# Which release images can be reused from the previous release instead of being rebuilt.
#
#   release-plan.sh VERSION [REF]        REF defaults to HEAD (the commit being released)
#
# Prints key=value lines (usable as $GITHUB_OUTPUT):
#   prev=0.2.10            the previous final release (the newest vX.Y.Z tag that is an ancestor of REF), empty if none
#   unchanged=bot          images whose inputs did not change since `prev` (space-separated; may be empty)
#
# An image listed in `unchanged` is published under the new version by re-tagging the previous release's image
# (same manifest, same layers), so the server finds every layer already on disk and pulls nothing.
#
# `api` is never listed: its binary carries the release version (`quests-server version`, /api/health).
# Any doubt (no previous release, a path that cannot be diffed) means "changed": a rebuild is always safe.
set -euo pipefail

version="${1:?usage: release-plan.sh VERSION [REF]}"
ref="${2:-HEAD}"

# What the `bot` image is built from (deploy/docker/Dockerfile: the python-base and bot stages).
bot_inputs=(
  src
  pyproject.toml
  uv.lock
  README.md
  deploy/docker/Dockerfile
  deploy/docker/entrypoint-bot.sh
  .dockerignore
)

prev_tag="$(git tag --merged "$ref" --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname \
  | grep -vE -- '-' | grep -vxF "v${version}" | head -n 1 || true)"

unchanged=()
if [ -n "$prev_tag" ]; then
  if git diff --quiet "$prev_tag" "$ref" -- "${bot_inputs[@]}"; then
    unchanged+=(bot)
  fi
fi

echo "prev=${prev_tag#v}"
echo "unchanged=${unchanged[*]:-}"
