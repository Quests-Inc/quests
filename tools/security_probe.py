#!/usr/bin/env python3
"""Attack a running Quests server the way an invited member would.

Needs PostgreSQL-backed workspaces and an OWNER account (the owner of the main
workspace). The probe invites a throw-away "attacker" into a fresh workspace and tries,
from there, to reach or change what belongs to the owner's workspace. Every check
prints PASS or FAIL; exit status 1 on any FAIL.

    python3 tools/security_probe.py --base http://127.0.0.1:8765 --owner alice

Standard library only (no uv, no venv). The password is asked for without echo, or read
from $QUESTS_PROBE_PASSWORD; it is deliberately not a command-line argument (shell
history, `ps`).

Run it against a scratch instance (it creates quests, workspaces and an account).
An owner with two-factor sign-in is asked for a code too (or $QUESTS_PROBE_CODE).
"""
from __future__ import annotations

import argparse
import base64
import getpass
import hashlib
import hmac
import http.cookiejar
import io
import json
import os
import re
import secrets
import struct
import sys
import time
import urllib.error
import urllib.request
import zlib

FAILS: list[str] = []

# Colour only on a terminal, and never when NO_COLOR is set (https://no-color.org).
COLOR = sys.stdout.isatty() and "NO_COLOR" not in os.environ


def paint(code: str, text: str) -> str:
    return f"\033[{code}m{text}\033[0m" if COLOR else text


def info(name: str, text: str) -> None:
    """A finding that is a known, accepted property rather than a pass/fail rule."""
    print(paint("1;33", "INFO") + " " + name + paint("33", f"  [{text}]"))


def check(name: str, ok: bool, detail: str = "") -> None:
    mark = paint("1;32", "PASS") if ok else paint("1;31", "FAIL")
    print(mark + " " + name + (paint("31", f"  [{detail}]") if detail and not ok else ""))
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


def totp(secret: str, step_offset: int = 0) -> str:
    key = base64.b32decode(secret.upper() + "=" * (-len(secret) % 8))
    h = hmac.new(key, struct.pack(">Q", int(time.time() // 30) + step_offset), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 10**6)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:8765")
    ap.add_argument("--owner", required=True)
    a = ap.parse_args()
    a.password = os.environ.get("QUESTS_PROBE_PASSWORD") or getpass.getpass(f"Password for {a.owner}: ")

    owner = Client(a.base)
    c, r, _ = owner.req("POST", "/api/auth/login", {"username": a.owner, "password": a.password})
    if c == 200 and isinstance(r, dict) and r.get("mfa_required"):
        # An account with two-factor sign-in: the password step only hands out a ticket.
        code = os.environ.get("QUESTS_PROBE_CODE") or getpass.getpass("Authenticator (or recovery) code: ")
        c, r, _ = owner.req("POST", "/api/auth/login/mfa", {"mfa_token": r.get("mfa_token", ""), "code": code.strip()})
    if c != 200:
        print("cannot log in as the owner:", c, r if isinstance(r, dict) else "")
        return 2
    tag = secrets.token_hex(3)
    c, ws, _ = owner.req("POST", "/api/workspaces", {"name": f"probe-{tag}"})
    if c != 201 or not isinstance(ws, dict) or "id" not in ws:
        print("cannot create the probe workspace:", c, ws)
        return 2
    c, inv, _ = owner.req("POST", f"/api/workspaces/{ws['id']}/invites")
    if c != 201 or "token" not in inv:
        print("cannot invite the attacker:", c, inv)
        return 2
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

    # 4. No way into the owner's workspace from outside (nothing is created: the call must fail).
    c, r, _ = evil.req("POST", "/api/workspaces/1/invites")
    check("a member of another workspace cannot invite into this one", c in (403, 404), str(c))
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

    # 5b. A note's freshness check is a command like a step's: same rules.
    c, enote, _ = evil.req("POST", "/api/notes", {"title": "probe note"})
    nid = enote["id"] if c == 201 else 0
    c, r, _ = evil.req("PUT", f"/api/notes/{nid}/check", {"command": "echo 1", "run_on": "lab"})
    check("member cannot set a note's check command", c == 403, str(c))
    c, r, _ = evil.req("PUT", f"/api/notes/{nid}/secrets/API_KEY", {"value": "x" * 8})
    check("member cannot set a note's secret", c == 403, str(c))
    owner.req("POST", f"/api/workspaces/{ws['id']}/switch")
    c, r, _ = owner.req("PUT", f"/api/notes/{nid}/check", {"command": "echo 1"})
    check("a note's check without a runner is refused (the server runs no commands)", c == 422, str(c))
    c, r, _ = owner.req("PUT", f"/api/notes/{nid}/check", {"command": "echo 0", "run_on": "lab"})
    check("a note's check on a runner is accepted for an owner", c == 200, str(c))
    owner.req("POST", "/api/workspaces/1/switch")

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
    # By design any signed-in client may post a heartbeat; the chips are global (not per workspace).
    info("members can post heartbeats (global overlay/telegram chips)", str(c))

    # 7. The assistant's endpoint is the user's to choose, not a way into this server's network.
    for url in ("https://127.0.0.1:8765/v1", "https://169.254.169.254/v1", "http://api.openai.com/v1"):
        c, r, _ = evil.req("PUT", "/api/me/llm", {"provider": "custom", "base_url": url, "model": "m", "api_key": "k" * 12})
        check(f"assistant endpoint {url} is refused", c == 422, str(c))
    c, r, _ = evil.req("GET", "/api/me/llm")
    check("assistant key is never returned", c == 200 and "api_key" not in json.dumps(r))

    # 8. Two-factor sign-in of an account (the attacker account stands in for any user).
    c, st, _ = evil.req("POST", "/api/me/2fa/setup")
    secret = st.get("secret", "") if c == 200 else ""
    c, en, _ = evil.req("POST", "/api/me/2fa/enable", {"code": totp(secret)}) if secret else (0, {}, None)
    check("2FA can be switched on with a correct code", c == 200 and len(en.get("recovery_codes", [])) == 10, str(c))
    step1 = Client(a.base)
    c, r, _ = step1.req("POST", "/api/auth/login", {"username": user, "password": "attacker-pass-1"})
    check("password alone no longer opens a session", c == 200 and r.get("mfa_required") and not list(step1.jar), str(r)[:80])
    c, _, _ = step1.req("GET", "/api/quests")
    check("no API access before the second factor", c == 401, str(c))
    ticket = r.get("mfa_token", "")
    c, _, _ = step1.req("POST", "/api/auth/login/mfa", {"mfa_token": ticket, "code": "000000"})
    check("a wrong code is refused", c == 401, str(c))
    c, _, _ = step1.req("POST", "/api/auth/login/mfa", {"mfa_token": ticket, "code": totp(secret)})
    check("the code accepted at enrolment cannot be replayed", c == 401, str(c))
    c, _, _ = Client(a.base).req("POST", "/api/auth/login/mfa", {"mfa_token": "forged-ticket", "code": totp(secret)})
    check("a forged ticket is refused", c == 401, str(c))
    c, _, _ = Client(a.base, token=None).req("GET", "/api/me/2fa")
    check("2FA state needs a session", c == 401, str(c))

    # 9. Hostile archive.
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
    owner.req("DELETE", f"/api/workspaces/{ws['id']}", {"confirm": ws["name"]})
    print("\n" + (paint("1;31", "%d failed" % len(FAILS)) if FAILS else paint("1;32", "all checks passed")))
    return 1 if FAILS else 0


if __name__ == "__main__":
    sys.exit(main())
