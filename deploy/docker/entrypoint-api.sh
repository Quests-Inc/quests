#!/bin/sh
set -eu
cd /app
mkdir -p /app/data
quests-server migrate
exec "$@"
