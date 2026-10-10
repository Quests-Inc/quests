#!/usr/bin/env python3
"""Reference queries for note search (quest=354): "before / after" in numbers, not in feelings.

Creates a small fixed corpus of notes in the workspace you point it at (and removes it again),
asks the search endpoint each question and checks that the expected note is where it should be.
Run it on a scratch instance:

    python3 tools/search_eval.py --base http://127.0.0.1:8765 --user alice

Password: prompt, or $QUESTS_PROBE_PASSWORD. Prints the rank of the expected note per query and the
share found at rank 1 / in the top 3. Exit status 1 when a query that must work does not.
"""
from __future__ import annotations

import argparse
import getpass
import http.cookiejar
import json
import os
import sys
import urllib.parse
import urllib.request

CORPUS = {
    "deploy": ("Деплой на прод", "Как выкатить сервис: запускается ci-deploy.sh, затем проверяем здоровье контейнеров. Откат через git reset."),
    "monitor": ("Мониторинг: Kuma, Prometheus, Grafana", "Метрики и дашборды. Алерты пока не нужны. Деплоится вместе со стеком при MONITORING=1."),
    "backup": ("Резервное копирование", "pg_dump раз в сутки, архив вложений уходит на WebDAV и на ПК по SFTP. Восстановление из дампа и бэкапы томов."),
    "quests": ("Квесты: модель данных", "Квесты состоят из шагов, у каждого шага может быть проверка. Квестлайны объединяют квесты."),
    "spaces": ("Пространства и доступы", "Участники приглашаются по ссылке; владелец может удалить пространство. Роли: владелец и участник."),
    "english": ("Notes about deployment", "How to deploy the stack and roll it back when something breaks."),
    "recipe": ("Рецепт борща", "Свёкла, капуста, говядина, картофель. Варить два часа."),
    "ssh": ("SSH и ключи доступа", "Подключение к серверу по ключу, настройка authorized_keys и ~/.ssh/config."),
}

# (query, key of the expected note, must-work)  — must-work queries fail the run when missed.
QUERIES = [
    ("деплой", "deploy", True),
    ("деплоить сервис", "deploy", True),          # another word form
    ("депл", "deploy", True),                      # prefix of the last word
    ("ltgkjq", "deploy", True),                    # "деплой" typed on a Latin layout
    ("квестов", "quests", True),                   # genitive of "квесты"
    ("бэкап", "backup", False),                    # a word from the body, not the title
    ("резервн копир", "backup", True),
    ("мониторнг", "monitor", True),                # typo in the title
    ("\"архив вложений\"", "backup", True),        # exact phrase
    ("как удалить пространство", "spaces", True),
    ("deploys", "english", True),                  # English word form
    ("рецепт", "recipe", True),
    ("authorized_keys", "ssh", True),              # a token with punctuation
    ("деплой -борщ", "deploy", True),              # exclusion does not hurt
    ("несуществующеесловоxyz", None, True),        # nothing found, nothing invented
]


class Client:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def req(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(self.base + path, data=data, method=method,
                                   headers={"Content-Type": "application/json", "Accept": "application/json"})
        try:
            with self.op.open(r, timeout=60) as resp:
                raw = resp.read()
                return resp.status, (json.loads(raw) if raw else None)
        except urllib.error.HTTPError as e:
            return e.code, None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:8765")
    ap.add_argument("--user", required=True)
    a = ap.parse_args()
    pw = os.environ.get("QUESTS_PROBE_PASSWORD") or getpass.getpass(f"Password for {a.user}: ")
    c = Client(a.base)
    code, _ = c.req("POST", "/api/auth/login", {"username": a.user, "password": pw})
    if code != 200:
        print("cannot log in:", code)
        return 2
    ids: dict[str, int] = {}
    for key, (title, body) in CORPUS.items():
        code, n = c.req("POST", "/api/notes", {"title": title, "description": body})
        if code != 201:
            print("cannot create a note:", code)
            return 2
        ids[key] = n["id"]
    failed = top1 = top3 = expected = 0
    try:
        for q, key, must in QUERIES:
            code, r = c.req("GET", "/api/notes/search?q=" + urllib.parse.quote(q) + "&limit=10")
            hits = [h["id"] for h in (r or {}).get("hits", [])]
            if key is None:
                ok = not [h for h in hits if h in ids.values()]
                print(("PASS " if ok else "FAIL ") + f"{q!r:34} -> nothing expected, found {len(hits)}")
                failed += 0 if ok else 1
                continue
            expected += 1
            pos = hits.index(ids[key]) + 1 if ids[key] in hits else 0
            top1 += pos == 1
            top3 += 0 < pos <= 3
            ok = pos == 1 if must else pos > 0
            print(("PASS " if ok else ("FAIL " if must else "WARN ")) + f"{q!r:34} -> rank {pos or '—'}")
            if must and not ok:
                failed += 1
    finally:
        for nid in ids.values():
            c.req("DELETE", f"/api/notes/{nid}")
    print(f"\nrank 1: {top1}/{expected}   top 3: {top3}/{expected}   failed: {failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
