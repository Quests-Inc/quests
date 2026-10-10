#!/usr/bin/env python3
"""emit_pool_command for the daily unread-mail quest template.

Reference copy — the copy that actually RUNS lives inline in the
template's emit_pool_command field (DB), pasted verbatim including this
shebang. quests' scheduler detects the shebang, writes it to a temp file
and executes it directly, so the script travels with the template row
instead of needing a file hand-deployed to wherever the server happens to
run (see execEmitPoolCommand in go/internal/schedule/materialize.go).
Keep this file and the template in sync by hand when either changes.

Logs into WorldClient (MDaemon webmail) the same way the web UI does —
IMAP (993/143) is closed on this server, HTTP is the only option — lists
unread messages in the inbox, and prints ONE quest object on stdout (shape
of POST /api/quests; the template supplies the defaults), one step per
unread message. No unread mail prints `null` = no quest this period.

There is no repeat protection: whatever is still unread is listed again
in the next period's quest.

Deliberately does NOT fetch message bodies: opening a message via
View=Message marks it read server-side (confirmed by hand — this is not
a browser-only quirk, the webmail and this script hit the same endpoint),
so pulling bodies here would silently mark every listed message read just
for a description preview. Title-only (from/subject/date) keeps the read
count on the actual mailbox honest; open the real message to read it.

See note=6 in the quests journal for the endpoint reference this is
built from (login/list/body-fetch quirks, non-strict-JSON parsing, etc).

Credentials come from this template's own secrets (set_template_secret /
PUT /api/templates/{id}/secrets/{key}), injected into the script's env only
while the scheduler runs it: MAIL_HOST, MAIL_USER, MAIL_PASSWORD. Never
hardcode them here — the script body is returned by the template API.
"""
import http.cookiejar
import json
import os
import re
import sys
import urllib.parse
import urllib.request


def env_or_die(name):
    v = os.environ.get(name, "").strip()
    if not v:
        print(f"missing {name} — set it as a template secret, see note=6", file=sys.stderr)
        sys.exit(1)
    return v


def make_opener():
    cj = http.cookiejar.CookieJar()
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))


def login(opener, host, user, password):
    url = f"https://{host}/WorldClient.dll?View=Main"
    data = urllib.parse.urlencode({"User": user, "Password": password}).encode()
    req = urllib.request.Request(url, data=data, method="POST")
    resp = opener.open(req, timeout=15)
    body = resp.read().decode("utf-8", errors="replace")
    m = re.search(r"Session=([A-Za-z0-9]+)", resp.geturl()) or re.search(
        r"Session=([A-Za-z0-9]+)", body
    )
    if not m:
        print("login failed: no Session id in response — check credentials", file=sys.stderr)
        sys.exit(1)
    return m.group(1)


def fetch_unread(opener, host, session_id):
    url = (
        f"https://{host}/WorldClient.dll?Session={session_id}&View=List"
        f"&ReturnJavaScript=1&FolderID=0&Sort=RevDate&Page=1&ContentType=javascript&UTF8=1"
    )
    # Must be POST — GET with the same params returns an empty body.
    req = urllib.request.Request(url, data=b"", method="POST")
    resp = opener.open(req, timeout=15)
    body = resp.read().decode("utf-8", errors="replace")

    messages = []
    for block in re.findall(r"\{[^{}]*\"id\"[^{}]*\}", body):

        def field(name, cast=str):
            fm = re.search(rf'"{name}"\s*:\s*"?([^",}}]*)"?', block)
            return cast(fm.group(1)) if fm else None

        messages.append(
            {
                "id": field("id"),
                "unread": field("unr", int) == 1,
                "from": field("frm"),
                "subject": field("sbj"),
                "date": field("dt"),
            }
        )
    return [m for m in messages if m["unread"]]


def main():
    host = env_or_die("MAIL_HOST")
    user = env_or_die("MAIL_USER")
    password = env_or_die("MAIL_PASSWORD")

    opener = make_opener()
    session_id = login(opener, host, user, password)
    unread = fetch_unread(opener, host, session_id)

    steps = [
        {
            "title": f"{m['from'] or '?'}: {m['subject'] or '(без темы)'}",
            "description": f"Получено: {m['date'] or '?'}",
        }
        for m in unread
        if m["id"]
    ]
    print(json.dumps({"steps": steps} if steps else None, ensure_ascii=False))


if __name__ == "__main__":
    main()
