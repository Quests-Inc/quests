#!/usr/bin/env python3
"""Attack a running Quests server the way an invited member would.

Needs PostgreSQL-backed workspaces and an OWNER account of a trusted workspace (the
main one). The probe invites a throw-away "attacker" into a fresh workspace and tries,
from there, to reach or change what belongs to the owner's workspace. Every check
prints PASS or FAIL; exit status 1 on any FAIL.

    python3 scripts/security_probe.py --base http://127.0.0.1:8765 --owner alice

Standard library only (no uv, no venv). The password is asked for without echo, or read
from $QUESTS_PROBE_PASSWORD; it is deliberately not a command-line argument (shell
history, `ps`).

Run it against a scratch instance (it creates quests, workspaces and an account).
"""
from __future__ import annotations

import argparse
import getpass
import http.cookiejar
import io
import json
import os
import re
import secrets
import sys
import urllib.error
import urllib.request
import zlib

FAILS: list[str] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    print(("PASS " if ok else "FAIL ") + name + (f"  [{detail}]" if detail and not ok else ""))
    if not ok:
        FAILS.append(name)


class Client:
    def __init__(self, base: str, token: str | None = None):
        self.base = base.rstrip("/")
        self.token = token
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def req(self, method, path, body=None, headers=None, ctype="application/json"):
        h = {"Accept": "application/json", **(headers or {})}
        if self.token:
            h["Authorization"] = "Bearer " + self.token
        data = None
        if body is not None:
            data = body if isinstance(body, bytes) else json.dumps(body).encode()
            h["Content-Type"] = ctype
        r = urllib.request.Request(self.base + path, data=data, method=method, headers=h)
        try:
            resp = self.opener.open(r, timeout=90)
            code, raw, hd = resp.status, resp.read(), resp.headers
        except urllib.error.HTTPError as e:
            code, raw, hd = e.code, e.read(), e.headers
        try:
            parsed = json.loads(raw) if raw and "json" in (hd.get("Content-Type") or "") else raw
        except ValueError:
            parsed = raw
        return code, parsed, hd


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:8765")
    ap.add_argument("--owner", required=True)
    a = ap.parse_args()
    a.password = os.environ.get("QUESTS_PROBE_PASSWORD") or getpass.getpass(f"Password for {a.owner}: ")

    owner = Client(a.base)
    c, _, _ = owner.req("POST", "/api/auth/login", {"username": a.owner, "password": a.password})
    if c != 200:
        print("cannot log in as the owner:", c)
        return 2
    tag = secrets.token_hex(3)
    c, ws, _ = owner.req("POST", "/api/workspaces", {"name": f"probe-{tag}"})
    c, inv, _ = owner.req("POST", f"/api/workspaces/{ws['id']}/invites")
    evil = Client(a.base)
    user = f"evil-{tag}"
    c, r, _ = evil.req("POST", f"/api/invites/{inv['token']}/accept", {"username": user, "password": "attacker-pass-1"})
    if c != 200:
        print("cannot create the attacker account:", c, r)
        return 2

    # Something worth stealing in the owner's main workspace.
    owner.req("POST", "/api/workspaces/1/switch")
    c, victim, _ = owner.req("POST", "/api/quests", {"title": f"VICTIM-{tag}", "description": "private"})
    vid = victim["id"]

    # 1. The assistant's loopback calls must stay in the caller's workspace.
    c, r, _ = evil.req("POST", "/api/llm/actions/apply",
                       {"batch": {"actions": [{"action": "update_quest", "quest_id": vid, "title": "PWNED"}]}})
    c2, q, _ = owner.req("GET", f"/api/quests/{vid}")
    check("LLM apply cannot touch another workspace", q["title"] == f"VICTIM-{tag}" and c != 200, f"{c} {q.get('title')}")

    # 2. PDF export must not read local files.
    html = '<html><body><link rel="attachment" href="file:///etc/hostname"><img src="file:///etc/hostname"></body></html>'
    c, pdf, _ = evil.req("POST", "/api/export/pdf", {"html": html})
    host = open("/etc/hostname", "rb").read().strip() if c == 200 else b""
    leaked = False
    if c == 200 and isinstance(pdf, bytes):
        for m in re.finditer(rb"stream\r?\n(.*?)\r?\nendstream", pdf, re.S):
            try:
                leaked = leaked or (host and host in zlib.decompress(m.group(1)))
            except zlib.error:
                pass
    check("PDF export does not embed local files", not leaked)

    # 3. Uploaded SVG icons are served sandboxed.
    c, ql, _ = evil.req("POST", "/api/questlines", {"title": "icon"})
    svg = b'<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>'
    mp = b'--X\r\nContent-Disposition: form-data; name="file"; filename="i.svg"\r\nContent-Type: image/svg+xml\r\n\r\n' + svg + b"\r\n--X--\r\n"
    evil.req("POST", f"/api/questlines/{ql['id']}/icon", mp, ctype="multipart/form-data; boundary=X")
    c, _, hd = evil.req("GET", f"/api/questlines/{ql['id']}/icon")
    check("SVG icon is sandboxed", "sandbox" in (hd.get("Content-Security-Policy") or "") and hd.get("X-Content-Type-Options") == "nosniff")

    # 4. No way into the workspace that can run commands on the host.
    c, r, _ = owner.req("POST", "/api/workspaces/1/invites")
    check("trusted workspace refuses invitations", c == 409, str(c))
    c, r, _ = evil.req("GET", "/api/quests", headers={"X-Quests-Workspace": "1"})
    check("header cannot name a foreign workspace", c == 403, str(c))
    c, r, _ = evil.req("POST", "/api/workspaces/1/switch")
    check("session cannot switch to a foreign workspace", c == 403, str(c))
    c, r, _ = evil.req("GET", f"/api/quests/{vid}")
    check("foreign quest by id is 404", c == 404, str(c))

    # 5. Members do not author commands, secrets or runners.
    c, r, _ = evil.req("POST", "/api/quests", {"title": "c", "steps": [{"title": "s", "check_command": "id", "run_on": "x"}]})
    check("member cannot set a command", c == 403, str(c))
    c, r, _ = evil.req("POST", "/api/runners", {"label": "mine"})
    check("member cannot add a runner", c == 403, str(c))
    c, mine, _ = evil.req("POST", "/api/quests", {"title": "mine"})
    c, r, _ = evil.req("PUT", f"/api/quests/{mine['id']}/secrets/API_KEY", {"value": "x"})
    check("member cannot set a secret", c == 403, str(c))
    c, r, _ = owner.req("PUT", f"/api/quests/{vid}/secrets/LD_PRELOAD", {"value": "x"})
    check("reserved secret names are refused", c == 422, str(c))

    # 6. Accounts and sessions.
    c, inv2, _ = owner.req("POST", f"/api/workspaces/{ws['id']}/invites")
    c, r, _ = Client(a.base).req("POST", f"/api/invites/{inv2['token']}/accept",
                                 {"username": "eve‮<img>", "password": "attacker-pass-1"})
    check("odd usernames are refused", c == 422, str(c))
    c, r, hd = Client(a.base).req("GET", "/")
    check("shell carries CSP and anti-framing headers",
          "script-src 'self'" in (hd.get("Content-Security-Policy") or "") and hd.get("X-Frame-Options") == "DENY")
    c, r, _ = evil.req("GET", "/api/health")
    check("health details are hidden from members",
          all(not (v or {}).get("detail") for v in (r.get("components") or {}).values()) if c == 200 else True)
    c, r, _ = evil.req("POST", "/api/health/heartbeat", {"component": "overlay", "detail": "fake"})
    check("members cannot post heartbeats", c == 403, str(c))

    # 7. Hostile archive.
    import zipfile
    bio = io.BytesIO()
    with zipfile.ZipFile(bio, "w") as z:
        z.writestr("manifest.json", json.dumps({"format": "quests-workspace", "version": 1, "workspace": "x", "files": False}))
        z.writestr("tables/quest.jsonl", json.dumps({"id": 1, "title": "q", "status": "active", "significance": "common",
                                                    "workspace_id": 1, "x); DROP TABLE quest;--": 1}))
    body = (b'--B\r\nContent-Disposition: form-data; name="file"; filename="a.zip"\r\nContent-Type: application/zip\r\n\r\n'
            + bio.getvalue() + b"\r\n--B--\r\n")
    c, r, _ = evil.req("POST", "/api/workspaces/import", body, ctype="multipart/form-data; boundary=B")
    check("archive with unknown columns is rejected", c == 422, str(c))

    # cleanup of the probe's own traces in the owner's workspace
    owner.req("DELETE", f"/api/quests/{vid}")
    owner.req("DELETE", f"/api/workspaces/{ws['id']}")
    print("\n%d failed" % len(FAILS) if FAILS else "\nall checks passed")
    return 1 if FAILS else 0


if __name__ == "__main__":
    sys.exit(main())
