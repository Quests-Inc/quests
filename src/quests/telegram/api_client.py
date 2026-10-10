"""Async HTTP client for local Quests API (no Telegram proxy)."""

from __future__ import annotations

import json
import os
from typing import Any

import aiohttp


class ApiError(Exception):
    def __init__(self, message: str, *, status: int | None = None) -> None:
        super().__init__(message)
        self.status = status


class QuestsApi:
    def __init__(
        self, base: str, session: aiohttp.ClientSession, token: str | None = None
    ) -> None:
        self.base = base.rstrip("/")
        self._session = session
        # Bearer token for an instance with accounts enabled; empty when open.
        self._token = (
            token if token is not None else (os.environ.get("QUESTS_API_TOKEN") or "")
        ).strip()

    async def request(
        self,
        method: str,
        path: str,
        *,
        body: dict[str, Any] | None = None,
        query: dict[str, Any] | None = None,
    ) -> Any:
        url = f"{self.base}{path}"
        params = {k: v for k, v in (query or {}).items() if v is not None} or None
        headers = {"X-Quests-Source": "telegram"}
        if self._token:
            headers["Authorization"] = f"Bearer {self._token}"
        try:
            async with self._session.request(
                method.upper(),
                url,
                json=body,
                params=params,
                headers=headers,
                timeout=aiohttp.ClientTimeout(total=30),
            ) as resp:
                raw = await resp.read()
                if resp.status >= 400:
                    detail = raw.decode("utf-8", errors="replace")
                    try:
                        parsed = json.loads(detail)
                        detail = str(parsed.get("detail", detail))
                    except (json.JSONDecodeError, TypeError, AttributeError):
                        pass
                    raise ApiError(f"API {resp.status}: {detail}", status=resp.status)
                if not raw:
                    return None
                return await resp.json(content_type=None)
        except aiohttp.ClientError as e:
            raise ApiError(
                f"не удалось связаться с API ({self.base}): {e}. "
                "Запусти сервер: just run-server"
            ) from e

    async def list_quests(self, *, status: str | None = "active") -> list[dict]:
        data = await self.request("GET", "/api/quests", query={"status": status})
        return list(data or [])

    async def get_quest(self, quest_id: int) -> dict:
        return await self.request("GET", f"/api/quests/{quest_id}")

    async def create_quest(self, body: dict[str, Any]) -> dict:
        return await self.request("POST", "/api/quests", body=body)

    async def patch_quest(self, quest_id: int, body: dict[str, Any]) -> dict:
        return await self.request("PATCH", f"/api/quests/{quest_id}", body=body)

    async def patch_step(
        self, quest_id: int, step_id: int, body: dict[str, Any]
    ) -> dict:
        return await self.request(
            "PATCH", f"/api/quests/{quest_id}/steps/{step_id}", body=body
        )

    async def list_categories(self) -> list[dict]:
        data = await self.request("GET", "/api/categories")
        return list(data or [])

    async def list_questlines(self) -> list[dict]:
        data = await self.request("GET", "/api/questlines")
        return list(data or [])

    async def preview_actions(self, text: str, *, no_clarify: bool = False) -> dict:
        """LLM action-batch dry-run (may take up to a few minutes)."""
        url = f"{self.base}/api/llm/actions/preview"
        headers = {"X-Quests-Source": "telegram"}
        if self._token:
            headers["Authorization"] = f"Bearer {self._token}"
        try:
            async with self._session.request(
                "POST",
                url,
                json={"text": text, "no_clarify": no_clarify},
                headers=headers,
                timeout=aiohttp.ClientTimeout(total=200),
            ) as resp:
                raw = await resp.read()
                if resp.status >= 400:
                    detail = raw.decode("utf-8", errors="replace")
                    try:
                        parsed = json.loads(detail)
                        detail = str(parsed.get("detail", detail))
                    except (json.JSONDecodeError, TypeError, AttributeError):
                        pass
                    raise ApiError(f"API {resp.status}: {detail}", status=resp.status)
                return await resp.json(content_type=None)
        except aiohttp.ClientError as e:
            raise ApiError(
                f"не удалось связаться с API ({self.base}): {e}"
            ) from e

    async def apply_actions(self, batch: dict[str, Any]) -> dict:
        return await self.request(
            "POST",
            "/api/llm/actions/apply",
            body={"batch": batch},
        )

    async def events_since(self, since: int) -> dict:
        return await self.request("GET", "/api/events", query={"since": since})

    async def sync_revision(self) -> int:
        data = await self.request("GET", "/api/sync")
        return int((data or {}).get("revision") or 0)

    async def heartbeat(self, *, component: str = "telegram", detail: str = "") -> None:
        await self.request(
            "POST",
            "/api/health/heartbeat",
            body={"component": component, "detail": detail},
        )
