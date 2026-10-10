#!/usr/bin/env bash
# Publish the production deployment of one release: make the `prod` branch of REMOTE hold this directory
# plus version.env pinning the release. doco-cd on the server polls that branch, so this push IS the deploy.
#
#   deploy/prod/publish.sh 1.4.0 git@github.com:owner/quests.git [prod]
#
# The branch is its own history (an orphan: no application source, only what the server needs), one commit
# per release; a rollback is publishing an older version again. Never force-pushes. Run by
# .github/workflows/release.yml; this directory is written so that it can move to a repository of its own.
set -euo pipefail

version="${1:?usage: publish.sh VERSION REMOTE_URL [BRANCH]}"
remote="${2:?usage: publish.sh VERSION REMOTE_URL [BRANCH]}"
branch="${3:-prod}"
src="$(cd "$(dirname "$0")" && pwd)"

if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "publish: '$version' is not a release version (MAJOR.MINOR.PATCH, no prerelease)" >&2
  exit 2
fi
# Secrets: either SOPS-encrypted in secrets.enc.env, or (fallback) in the host's legacy .env, which doco-cd
# mounts as /legacy.env. A secrets.enc.env that is not really encrypted must never be published.
if [ -f "$src/secrets.enc.env" ]; then
  if grep -q REPLACE_WITH "$src/.sops.yaml" || ! grep -q 'ENC\[' "$src/secrets.enc.env" || ! grep -q '^sops_' "$src/secrets.enc.env"; then
    echo "publish: secrets.enc.env is not SOPS-encrypted: run 'just secrets-init' (or delete it to use the host's legacy .env)" >&2
    exit 2
  fi
else
  echo "publish: no secrets.enc.env: the stack will read secrets from the host's legacy .env only" >&2
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
git init -q "$work"
cd "$work"
git remote add origin "$remote"
if git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
  git fetch -q origin "$branch"
  git checkout -q -B "$branch" FETCH_HEAD
  git rm -rq --ignore-unmatch . # the tree is replaced as a whole; history stays
else
  git checkout -q --orphan "$branch"
fi

cp -a "$src/." "$work/"
rm -f "$work/publish.sh"
printf 'QUESTS_VERSION=%s\n' "$version" >"$work/version.env"

git add -A
if git diff --cached --quiet && git rev-parse -q --verify HEAD >/dev/null; then
  echo "publish: $branch already deploys $version"
  exit 0
fi
git -c user.name="quests-release" -c user.email="quests-release@users.noreply.github.com" \
  commit -q -m "release v$version" -m "source: ${GITHUB_SHA:-local}"
git push -q origin "$branch"
echo "publish: $branch -> v$version ($(git rev-parse --short HEAD))"
