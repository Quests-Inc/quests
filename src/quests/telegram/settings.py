"""Telegram bot settings from env + CLI."""

from __future__ import annotations

import argparse
import os
import socket
from dataclasses import dataclass
from urllib.parse import urlparse

from quests.config import DATA_DIR, HOST, PORT

DEFAULT_PROXY = "http://127.0.0.1:12334"
DEFAULT_API = f"http://{HOST}:{PORT}"


@dataclass(frozen=True)
class TgSettings:
    token: str
    user_ids: frozenset[int]
    proxy: str
    api_base: str
    chats_path: str
    dedup_path: str


def _parse_user_ids(raw: str | None) -> frozenset[int]:
    if not raw or not raw.strip():
        return frozenset()
    out: set[int] = set()
    for part in raw.replace(";", ",").split(","):
        part = part.strip()
        if not part:
            continue
        out.add(int(part))
    return frozenset(out)


def check_proxy_reachable(proxy_url: str, *, timeout: float = 2.0) -> None:
    """Fail fast if proxy host:port does not accept TCP."""
    parsed = urlparse(proxy_url)
    host = parsed.hostname
    port = parsed.port
    if not host or not port:
        raise SystemExit(
            f"некорректный прокси URL (нужен host:port): {proxy_url!r}"
        )
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return
    except OSError as e:
        raise SystemExit(
            f"прокси недоступен ({proxy_url}): {e}. "
            "Подними прокси или укажи другой --proxy / QUESTS_TG_PROXY"
        ) from e


def build_settings(argv: list[str] | None = None) -> TgSettings:
    parser = argparse.ArgumentParser(
        prog="quests-telegram",
        description="Telegram-бот Quests (список / статус / уведомления)",
    )
    parser.add_argument(
        "--token",
        default=os.environ.get("QUESTS_TG_TOKEN") or "",
        help="токен бота (или QUESTS_TG_TOKEN)",
    )
    parser.add_argument(
        "--proxy",
        default=os.environ.get("QUESTS_TG_PROXY") or DEFAULT_PROXY,
        help=f"HTTP(S)/SOCKS прокси для Telegram API (дефолт {DEFAULT_PROXY})",
    )
    parser.add_argument(
        "--api",
        default=(os.environ.get("QUESTS_API") or DEFAULT_API).rstrip("/"),
        help=f"база локального Quests API (дефолт {DEFAULT_API})",
    )
    parser.add_argument(
        "--users",
        default=os.environ.get("QUESTS_TG_USER_IDS") or "",
        help="whitelist Telegram user id через запятую (или QUESTS_TG_USER_IDS)",
    )
    parser.add_argument(
        "--skip-proxy-check",
        action="store_true",
        help="не проверять TCP до прокси при старте",
    )
    ns = parser.parse_args(argv)

    token = (ns.token or "").strip()
    if not token:
        raise SystemExit(
            "нужен токен бота: --token или QUESTS_TG_TOKEN"
        )

    proxy = (ns.proxy or "").strip()
    if not proxy:
        raise SystemExit(
            "нужен прокси: --proxy или QUESTS_TG_PROXY "
            f"(дефолт {DEFAULT_PROXY})"
        )

    user_ids = _parse_user_ids(ns.users)
    if not user_ids:
        raise SystemExit(
            "нужен whitelist: --users или QUESTS_TG_USER_IDS "
            "(узнать id: напиши боту /start после временного ослабления — "
            "или @userinfobot)"
        )

    if not ns.skip_proxy_check:
        check_proxy_reachable(proxy)

    DATA_DIR.mkdir(parents=True, exist_ok=True)
    return TgSettings(
        token=token,
        user_ids=user_ids,
        proxy=proxy,
        api_base=str(ns.api).rstrip("/"),
        chats_path=str(DATA_DIR / "telegram_chats.json"),
        dedup_path=str(DATA_DIR / "telegram_notify.json"),
    )
