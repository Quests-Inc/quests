#!/usr/bin/env python3
"""A note's freshness check for notes that describe files in a git repository (quest=370).

Put the commit the note was last verified against in the note's text:

    <!-- verified: 16189db -->

and give the note this check (it runs wherever the repository is checked out — a runner on that
machine, or the server):

    python3 scripts/note_check_git.py --repo ~/Documents/projects/Quests \\
        go/cmd/quests-server/admin.go deploy/docker/ci-deploy.sh

It prints {"outdated": 0|1, "reason": "…"}. The note is out of date when any of the paths has a newer
commit than the marker. To mark it fresh again, edit the marker to the commit it now describes
(`git log -1 --format=%h -- <paths>`) — no other state anywhere.

The note is read from the API ($QUESTS_API, $QUESTS_API_TOKEN, $QUESTS_NOTE_ID and, for a non-default
workspace, $QUESTS_WORKSPACE — the runner and the server set the note id and workspace themselves).
A missing marker, an unknown commit or an unreachable API are errors (exit 2): the verdict stays.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.request

MARKER = re.compile(r"verified:\s*([0-9a-fA-F]{7,40})")


def fail(msg: str) -> "NoReturn":  # noqa: F821
    print(msg, file=sys.stderr)
    sys.exit(2)


def read_note() -> str:
    nid = os.environ.get("QUESTS_NOTE_ID", "").strip()
    if not nid.isdigit():
        fail("QUESTS_NOTE_ID is not set (the check runs from a note's check)")
    base = os.environ.get("QUESTS_API", "http://127.0.0.1:8765").rstrip("/")
    headers = {"Accept": "application/json", "X-Quests-Source": "check"}
    if os.environ.get("QUESTS_API_TOKEN"):
        headers["Authorization"] = "Bearer " + os.environ["QUESTS_API_TOKEN"]
    if os.environ.get("QUESTS_WORKSPACE"):
        headers["X-Quests-Workspace"] = os.environ["QUESTS_WORKSPACE"]
    req = urllib.request.Request(f"{base}/api/notes/{nid}", headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return json.load(resp).get("description", "")
    except Exception as e:  # noqa: BLE001 — any failure is "cannot tell"
        fail(f"cannot read note {nid}: {e}")


def git(repo: str, *args: str) -> str:
    try:
        return subprocess.run(["git", "-C", repo, *args], check=True, capture_output=True, text=True, timeout=30).stdout.strip()
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError) as e:
        detail = getattr(e, "stderr", "") or str(e)
        fail(f"git {' '.join(args[:2])}: {detail.strip()[:200]}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", default=".", help="the git repository the note describes")
    ap.add_argument("paths", nargs="+", help="files or directories (relative to --repo) the note depends on")
    a = ap.parse_args()

    m = MARKER.search(read_note())
    if not m:
        fail("в заметке нет маркера <!-- verified: <коммит> -->")
    verified = m.group(1)
    known = subprocess.run(["git", "-C", a.repo, "cat-file", "-e", verified + "^{commit}"], capture_output=True, timeout=30)
    if known.returncode != 0:
        fail(f"коммит {verified} неизвестен репозиторию {a.repo}")
    # Commits touching the paths after the verified one.
    newer = git(a.repo, "log", "--format=%h", f"{verified}..HEAD", "--", *a.paths).splitlines()
    if not newer:
        print(json.dumps({"outdated": 0}))
        return 0
    latest = git(a.repo, "log", "-1", "--format=%h %s", "--", *a.paths)
    reason = f"{len(newer)} новых коммит(ов) в {', '.join(a.paths)} после {verified[:7]}; последний: {latest}"
    print(json.dumps({"outdated": 1, "reason": reason}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
