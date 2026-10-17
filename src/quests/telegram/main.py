"""Entry: quests-telegram — polling bot via proxy + local Quests API."""

from __future__ import annotations

import asyncio
import logging
import sys

import aiohttp
from aiogram import Bot, Dispatcher
from aiogram.client.default import DefaultBotProperties
from aiogram.client.session.aiohttp import AiohttpSession
from aiogram.enums import ParseMode
from aiogram.fsm.storage.memory import MemoryStorage

from quests.telegram.api_client import QuestsApi
from quests.telegram.handlers import build_router
from quests.telegram.notify import events_loop, window_start_loop
from quests.telegram.settings import TgSettings, build_settings
from quests.telegram.store import ChatRegistry, NotifyDedup

log = logging.getLogger("quests.telegram")


async def _run(settings: TgSettings) -> None:
    session = AiohttpSession(proxy=settings.proxy)
    bot = Bot(
        token=settings.token,
        session=session,
        default=DefaultBotProperties(parse_mode=ParseMode.HTML),
    )
    dp = Dispatcher(storage=MemoryStorage())

    api_http = aiohttp.ClientSession()
    api = QuestsApi(settings.api_base, api_http)
    chats = ChatRegistry(settings.chats_path)
    dedup = NotifyDedup(settings.dedup_path)

    dp.include_router(build_router(api=api, settings=settings, chats=chats))

    notify_events = asyncio.create_task(
        events_loop(bot, api, chats, dedup, settings),
        name="tg-events",
    )
    notify_windows = asyncio.create_task(
        window_start_loop(bot, api, chats, dedup, settings),
        name="tg-windows",
    )

    log.info(
        "telegram bot starting (proxy=%s api=%s users=%s)",
        settings.proxy,
        settings.api_base,
        ",".join(str(u) for u in sorted(settings.user_ids)),
    )
    try:
        # Drop pending updates so restart does not replay old callbacks.
        await dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types())
    finally:
        notify_events.cancel()
        notify_windows.cancel()
        for t in (notify_events, notify_windows):
            try:
                await t
            except asyncio.CancelledError:
                pass
        await api_http.close()
        await bot.session.close()


def main(argv: list[str] | None = None) -> None:
    from quests.envload import load_dotenv_files

    loaded = load_dotenv_files()
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        stream=sys.stderr,
    )
    if loaded is not None:
        log.info("loaded env from %s", loaded)
    settings = build_settings(argv)
    try:
        asyncio.run(_run(settings))
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
