#!/usr/bin/env bash
# Build the check runner (runs on the device that executes checks) → go/bin/quests-runner
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${QUESTS_RUNNER_OUT:-$ROOT/go/bin/quests-runner}"
VERSION="${QUESTS_RUNNER_VERSION:-$(git -C "$ROOT" describe --always --dirty 2>/dev/null || echo dev)}"
mkdir -p "$(dirname "$OUT")"
cd "$ROOT/go"
echo "==> building $OUT ($VERSION)"
CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$VERSION" -o "$OUT" ./cmd/quests-runner
echo "    ok: $OUT"
