"""Quests MCP (stdio) — journal tools over the HTTP API.

Global Cursor config (``~/.cursor/mcp.json``) so any workspace can use it::

    {
      "mcpServers": {
        "quests": {
          "command": "/home/YOU/.local/bin/uv",
          "args": [
            "run",
            "--directory",
            "/home/YOU/Quests",
            "quests-mcp"
          ],
          "env": {
            "QUESTS_API": "http://SERVER:8765",
            "QUESTS_API_TOKEN": "<from quests-server token add mcp-cursor>"
          }
        }
      }
    }

Local open API: omit ``QUESTS_API_TOKEN`` and point ``QUESTS_API`` at loopback.
Or: ``QUESTS_API=… QUESTS_API_TOKEN=… uv run --directory ~/Quests quests-mcp``.
"""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any

from mcp.server.mcpserver import MCPServer

from quests.config import HOST, PORT
from quests.envload import load_dotenv_files
from quests.refs import resolve_category_id as _shared_resolve_category_id
from quests.refs import resolve_questline_id as _shared_resolve_questline_id

_SIGNIFICANCES = ("insignificant", "common", "uncommon", "epic", "legendary")


def _check_significance(value: str) -> str:
    if value not in _SIGNIFICANCES:
        raise ValueError(f"significance must be one of: {', '.join(_SIGNIFICANCES)} (got {value!r})")
    return value


API_BASE = (os.environ.get("QUESTS_API") or f"http://{HOST}:{PORT}").rstrip("/")

# IDs of your own vault notes holding the agent guide / session-report parent.
# Unset (default) → the instructions skip the pointer instead of citing a note
# that doesn't exist in a fresh install.
AGENT_GUIDE_NOTE_ID = os.environ.get("QUESTS_AGENT_GUIDE_NOTE_ID")
SESSION_REPORTS_NOTE_ID = os.environ.get("QUESTS_SESSION_REPORTS_NOTE_ID")

ALLOWED_STATUS = {"active", "expired", "frozen", "completed", "failed", "archived"}

_guide_pointer = (
    (
        f"Before your first non-trivial action in this project, read "
        f"note={AGENT_GUIDE_NOTE_ID} (get_note_context) for the data model, "
        f"workflow expectations (interim reports as notes under "
        f"note={SESSION_REPORTS_NOTE_ID or '…'}, splitting work into real steps, "
        f"closing steps only once actually verified, cross-linking via ref "
        f"tokens), and tone guidance — do not skip this. "
    )
    if AGENT_GUIDE_NOTE_ID
    else ""
)

server = MCPServer(
    "quests",
    instructions=(
        "Quests is a personal task/quest journal (significance levels) "
        "for one user, conversing in Russian — a web "
        "journal, desktop GTK/Wayland HUD, Telegram bot, and Android app all "
        "hit the same REST API this MCP server wraps. "
        + _guide_pointer +
        "Use list_questlines then list_quests to browse "
        "missions; list_notes for the knowledge vault. get_base_context for one "
        "entity's own fields (quest/step/questline/template — accepts a pasted ref like "
        "quest=23 / step=252 / questline=3 / template=5); get_note_context for a note's own "
        "fields plus refs/backlinks/children (e.g. note=5). get_active_context for "
        "live/pending work; get_inactive_context for completed/failed/archived "
        "history with full steps — together they cover every quest, full detail. "
        "Notes are markdown knowledge pages (toolkits, facts) — not quests. "
        "They are not owned by a questline. Link them from a quest/step/"
        "questline description with note=N; notes link back with quest=N / "
        "note=N. Put scripts/config in the note body (fenced code), do not "
        "attach the same zip to every mission. Attachments are binary "
        "artifacts (PDF, images), not toolkits. "
        "To create a new quest use create_quest (title, optional steps inline, "
        "deadline_at + duration_seconds for a reminder window that opens "
        "duration_seconds before deadline_at, automated=true to demote create/start "
        "overlay toasts, category/questline by id or by name — e.g. "
        "category='health', questline='Сайт Рефкул'). "
        "Steps may include check_command (one line, several lines, or a script starting with #!/usr/bin/env …; it runs on a runner, so run_on=<runner label> is required with it; several labels separated by commas = try them in order, the first one that is online runs it — only when the command does not depend on one particular device), run_mode=poll|once|watch, wait_previous. "
        "To create a new questline (project/theme container) use create_questline. "
        "Tags (work-surface labels like frontend/api/mcp): list_tags first; "
        "create_tag only if nothing suitable exists; set_quest_tags / tag_ids on "
        "create_quest to attach (max 5). Prefer label ≤6 chars (front, api, infra) — "
        "sidebar bookmarks; abbreviate freely. Do not invent near-duplicate slugs. "
        "To create a knowledge page use create_note (title, markdown "
        "description, optional parent_id for the notes tree). Images in markdown "
        "come only from attachments: upload_attachment first, then "
        "![alt](attachment=N) — raster only (png/jpeg/gif/webp); append |center, "
        "|left or |right to alt to align (![alt|center](attachment=N)); external URLs, "
        "data: URIs and raw <img> are stripped by the web renderer. Notes flagged "
        "CATEGORY are folders: list_note_categories shows them, "
        "list_notes(category_id=…, brief=true) browses one, and create_note "
        "without parent_id is refused until you pick a category or pass "
        "no_category=true. "
        "Quest/step/note `description` is markdown in the journal (lists, links, "
        "code, emphasis) — use it; title stays plain. HUD/Telegram show quest "
        "text raw and ignore notes. "
        "To change steps on an existing quest use add_step / update_step / "
        "delete_step (do not replace the whole steps array). "
        "To change quest lifecycle or metadata use update_quest "
        "(status: active|expired|frozen|completed|failed|archived; pin; title; …) — "
        "do not curl the Quests API or dig into the Quests repo for that. "
        "Attachments: list_quests and the get_*_context tools return metadata only "
        "(filename, size, type, scan_status, comment, available, "
        "source_updated, revision, revision_count) — never file bytes. Use "
        "get_attachment when you actually need the contents of one file "
        "(optional revision=N for an older version). Use upload_attachment to "
        "add a local file — or pass as_version_of=attachment_id to upload a "
        "new revision of an existing card instead of a sibling *_v2 file. "
        "Needs the API's WebDAV storage and ClamAV scanning configured "
        "server-side, and fails otherwise. "
        "set_attachment_hidden hides/restores a card (soft delete: hidden files "
        "vanish from lists and contexts, remain on the web Attachments tab). "
        "If you're blocked on the user's input and they may not be watching this "
        "conversation, use ping_user — it's the only tool guaranteed to interrupt "
        "them via the overlay HUD."
    ),
)


def _api(
    method: str,
    path: str,
    *,
    query: dict[str, Any] | None = None,
    body: dict[str, Any] | None = None,
) -> Any:
    url = f"{API_BASE}{path}"
    if query:
        q = {k: v for k, v in query.items() if v is not None}
        if q:
            url = f"{url}?{urllib.parse.urlencode(q)}"
    data = None
    headers = {"Accept": "application/json", "X-Quests-Source": "mcp"}
    # Bearer token for an instance with accounts enabled; empty when the API is open.
    token = (os.environ.get("QUESTS_API_TOKEN") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method.upper())
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
            return json.loads(raw.decode("utf-8")) if raw else None
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(detail)
            detail = parsed.get("detail", detail)
        except json.JSONDecodeError:
            pass
        raise RuntimeError(f"API {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(
            f"cannot reach Quests API ({API_BASE}): {e.reason}"
        ) from e


def _api_raw(
    method: str,
    path: str,
    *,
    query: dict[str, Any] | None = None,
) -> tuple[bytes, dict[str, str]]:
    url = f"{API_BASE}{path}"
    if query:
        q = {k: v for k, v in query.items() if v is not None}
        if q:
            url = f"{url}?{urllib.parse.urlencode(q)}"
    headers = {"Accept": "*/*"}
    token = (os.environ.get("QUESTS_API_TOKEN") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, headers=headers, method=method.upper())
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
            hdrs = {k.lower(): v for k, v in resp.headers.items()}
            return raw, hdrs
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(detail)
            detail = parsed.get("detail", detail)
        except json.JSONDecodeError:
            pass
        raise RuntimeError(f"API {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(
            f"cannot reach Quests API ({API_BASE}): {e.reason}"
        ) from e


def _api_get(path: str, query: dict[str, Any] | None = None) -> Any:
    return _api("GET", path, query=query)


def _api_upload_file(
    path: str,
    file_path: str,
    *,
    field: str = "file",
    extra_fields: dict[str, str] | None = None,
) -> Any:
    """POST a local file as multipart/form-data (icon uploads, attachments, ...)."""
    try:
        with open(file_path, "rb") as fh:
            raw = fh.read()
    except OSError as e:
        raise ValueError(f"cannot read {file_path}: {e}") from e
    filename = os.path.basename(file_path)
    content_type = mimetypes.guess_type(filename)[0] or "application/octet-stream"
    boundary = uuid.uuid4().hex
    parts = bytearray()
    for name, value in (extra_fields or {}).items():
        if value is None:
            continue
        parts += (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="{name}"\r\n\r\n'
            f"{value}\r\n"
        ).encode()
    parts += (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="{field}"; filename="{filename}"\r\n'
        f"Content-Type: {content_type}\r\n\r\n"
    ).encode() + raw + f"\r\n--{boundary}--\r\n".encode()
    body = bytes(parts)
    url = f"{API_BASE}{path}"
    headers = {
        "Accept": "application/json",
        "Content-Type": f"multipart/form-data; boundary={boundary}",
    }
    token = (os.environ.get("QUESTS_API_TOKEN") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw_resp = resp.read()
            return json.loads(raw_resp.decode("utf-8")) if raw_resp else None
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(detail)
            detail = parsed.get("detail", detail)
        except json.JSONDecodeError:
            pass
        raise RuntimeError(f"API {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(
            f"cannot reach Quests API ({API_BASE}): {e.reason}"
        ) from e


def _attachments_brief(owner_type: str, owner_id: int) -> list[dict[str, Any]]:
    seg = {"questline": "questlines", "note": "notes"}.get(owner_type, "quests")
    try:
        rows = _api_get(f"/api/{seg}/{owner_id}/attachments", {"stat": "0"}) or []
    except RuntimeError:
        # Route missing on an older API, or owner gone — listing quests
        # should still work.
        return []
    out: list[dict[str, Any]] = []
    for a in rows:
        if a.get("hidden"):
            continue  # hidden cards are for the web Attachments tab only
        out.append(
            {
                "id": a.get("id"),
                "filename": a.get("filename"),
                "size_bytes": a.get("size_bytes"),
                "content_type": a.get("content_type_detected")
                or a.get("content_type_declared"),
                "scan_status": a.get("scan_status"),
                "comment": a.get("comment"),
                "available": a.get("available"),
                "source_updated": a.get("source_updated"),
            }
        )
    return out


def _tool_query(*, quiet: bool) -> dict[str, str]:
    q = {"source": "mcp"}
    if quiet:
        q["quiet"] = "1"
    return q


def _parse_ref(ref: str) -> tuple[str, int]:
    # Kinds the frontend and Go backend already parse out of description text
    # (frontend/src/lib/js/refs.js REF_RE, go/internal/refs/refs.go TokenRe) —
    # "attachment" was missing here, so ref="attachment=N" pasted straight
    # from a description raised "bad ref" instead of the guided error below.
    text = (ref or "").strip()
    for kind in ("questline", "quest", "step", "note", "attachment", "template"):
        prefix = f"{kind}="
        if text.startswith(prefix):
            return kind, int(text[len(prefix) :].strip())
    raise ValueError(
        f"bad ref {ref!r}; expected quest=N, step=N, questline=N, template=N, note=N, or attachment=N"
    )


def _quest_summary(q: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": q.get("id"),
        "title": q.get("title"),
        "status": q.get("status"),
        "significance": q.get("significance"),
        "pinned": q.get("pinned"),
        "overlay_hidden": q.get("overlay_hidden"),
        "failure_inbox": q.get("failure_inbox"),
        "progress_label": q.get("progress_label"),
        "steps_done": q.get("steps_done"),
        "steps_total": q.get("steps_total"),
        "category_id": q.get("category_id"),
        "category_slug": q.get("category_slug"),
        "category_label": q.get("category_label"),
        "questline_id": q.get("questline_id"),
        "questline_title": q.get("questline_title"),
        "deadline_at": q.get("deadline_at"),
        "updated_at": q.get("updated_at"),
    }


def _steps_brief(q: dict[str, Any]) -> list[dict[str, Any]]:
    out = []
    for s in q.get("steps") or []:
        row = {
            "id": s.get("id"),
            "title": s.get("title"),
            "progress_current": s.get("progress_current"),
            "progress_total": s.get("progress_total"),
            "done": s.get("done"),
            "sort_order": s.get("sort_order"),
        }
        desc = s.get("description")
        if desc:
            row["description"] = desc
        out.append(row)
    return out


def _resolve_category_id(raw: str | int | None) -> int | None:
    return _shared_resolve_category_id(raw, api_get=_api_get)


def _resolve_questline_id(raw: str | int | None) -> int | None:
    return _shared_resolve_questline_id(raw, api_get=_api_get)


def _quest_mutation_result(q: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": q.get("id"),
        "title": q.get("title"),
        "status": q.get("status"),
        "pinned": q.get("pinned"),
        "overlay_hidden": q.get("overlay_hidden"),
        "significance": q.get("significance"),
        "progress_label": q.get("progress_label"),
        "questline_id": q.get("questline_id"),
        "category_id": q.get("category_id"),
        "steps": _steps_brief(q),
    }


def _step_body(s: dict[str, Any]) -> dict[str, Any]:
    body: dict[str, Any] = {
        "title": s["title"],
        "description": s.get("description", ""),
        "progress_total": max(1, int(s.get("progress_total", 1))),
        "progress_current": max(0, int(s.get("progress_current", 0))),
    }
    if s.get("sort_order") is not None:
        body["sort_order"] = int(s["sort_order"])
    if s.get("check_command") is not None:
        body["check_command"] = s["check_command"]
    if s.get("check_interval_seconds") is not None:
        body["check_interval_seconds"] = int(s["check_interval_seconds"])
    if s.get("wait_previous") is not None:
        body["wait_previous"] = bool(s["wait_previous"])
    if s.get("run_mode") is not None:
        body["run_mode"] = str(s["run_mode"])
    if s.get("run_on") is not None:
        body["run_on"] = str(s["run_on"])
    return body


@server.tool(
    description=(
        "Full context for one note: its own description plus refs (quest=N/"
        "note=N tokens it cites) and backlinks (other quests/notes that cite "
        "it) and children (nested notes). This is the only tool that resolves "
        "note refs/backlinks — quests/steps/questlines don't need it, use "
        "get_base_context or get_active_context/get_inactive_context for those. "
        "A note the user marked PRIVATE (is_private=true) comes back with "
        "description replaced by a placeholder — that's deliberate (see "
        "note=82), not an error; don't retry, and don't fetch it another way "
        "(e.g. curl) to work around it — if the content is actually needed, "
        "ask the user to paste it or read it themselves. The note also carries `outdated` / "
        "`outdated_reason` (a stale note may be inaccurate — verify it against the code before relying "
        "on it, and mark it with set_note_outdated if it is wrong) and `check` (its freshness-check "
        "command, interval, last run and error)."
    )
)
def get_note_context(note_id: int) -> dict[str, Any]:
    return _api_get("/api/context", {"note": note_id})


@server.tool(
    description=(
        "The object's own direct fields, no siblings/linked_notes bloat. "
        "For a questline: description, attachments, and its quests (id/title/"
        "status/significance/category/...) across every status. For a quest: "
        "description, steps, attachments, and a brief questline (if any). For "
        "a step: its description/progress and a brief owning quest. Pass "
        "exactly one of ref / quest / step / questline (notes aren't "
        "supported here — use get_note_context for those; attachments "
        "aren't either — use get_attachment)."
    )
)
def get_base_context(
    ref: str | None = None,
    quest: int | None = None,
    step: int | None = None,
    questline: int | None = None,
) -> dict[str, Any]:
    if ref:
        kind, eid = _parse_ref(ref)
        if kind == "template":
            return _api_get(f"/api/templates/{eid}")
        if kind == "note":
            raise ValueError("get_base_context does not support notes; use get_note_context")
        if kind == "attachment":
            raise ValueError(
                "get_base_context does not support attachments; use get_attachment "
                "(it also needs the owning quest=/questline=/note=)"
            )
    else:
        chosen = [
            (k, v)
            for k, v in (("quest", quest), ("step", step), ("questline", questline))
            if v is not None
        ]
        if len(chosen) != 1:
            raise ValueError("provide exactly one of: ref, quest, step, questline")
        kind, eid = chosen[0]

    if kind == "questline":
        resp = _api_get("/api/context", {"questline": eid})
        line = resp.get("questline") or {}
        return {
            "id": line.get("id"),
            "title": line.get("title"),
            "description": line.get("description"),
            "color": line.get("color"),
            "icon": line.get("icon"),
            "icon_url": line.get("icon_url"),
            "category_id": line.get("category_id"),
            "category_slug": line.get("category_slug"),
            "category_label": line.get("category_label"),
            "attachments": (resp.get("attachments") or {}).get("questline", []),
            "quests": [_quest_summary(q) for q in resp.get("quests") or []],
        }

    if kind == "quest":
        resp = _api_get("/api/context", {"quest": eid})
        q = next((r for r in resp.get("quests") or [] if r.get("id") == eid), None)
        if q is None:
            raise ValueError(f"quest={eid} not found")
        line = resp.get("questline")
        out: dict[str, Any] = {
            "id": q.get("id"),
            "title": q.get("title"),
            "description": q.get("description"),
            "status": q.get("status"),
            "significance": q.get("significance"),
            "pinned": q.get("pinned"),
            "overlay_hidden": q.get("overlay_hidden"),
            "category_id": q.get("category_id"),
            "category_slug": q.get("category_slug"),
            "category_label": q.get("category_label"),
            "deadline_at": q.get("deadline_at"),
            "created_at": q.get("created_at"),
            "updated_at": q.get("updated_at"),
            "steps": _steps_brief(q),
            "attachments": ((resp.get("attachments") or {}).get("by_quest") or {}).get(
                str(eid), []
            ),
            "questline": None,
        }
        if line:
            out["questline"] = {
                "id": line.get("id"),
                "title": line.get("title"),
                "color": line.get("color"),
                "icon": line.get("icon"),
                "category_id": line.get("category_id"),
                "category_label": line.get("category_label"),
            }
        return out

    # step
    resp = _api_get("/api/context", {"step": eid})
    owner = None
    step_row = None
    for q in resp.get("quests") or []:
        for s in q.get("steps") or []:
            if s.get("id") == eid:
                owner, step_row = q, s
                break
        if owner:
            break
    if owner is None:
        raise ValueError(f"step={eid} not found")
    return {
        "step": {
            "id": step_row.get("id"),
            "title": step_row.get("title"),
            "description": step_row.get("description"),
            "progress_current": step_row.get("progress_current"),
            "progress_total": step_row.get("progress_total"),
            "done": step_row.get("done"),
            "sort_order": step_row.get("sort_order"),
        },
        "quest": _quest_summary(owner),
    }


def _quests_by_status_context(
    statuses: tuple[str, ...],
    questline: str | int | None,
    *,
    all_steps: bool,
) -> dict[str, Any]:
    qline_id = _resolve_questline_id(questline) if questline is not None else None
    rows: list[dict[str, Any]] = []
    for st in statuses:
        rows.extend(_api_get("/api/quests", {"status": st}) or [])
    if qline_id is not None:
        rows = [q for q in rows if q.get("questline_id") == qline_id]

    quests_out = []
    for row in rows:
        qid = row.get("id")
        if qid is None:
            continue
        resp = _api_get("/api/context", {"quest": qid})
        q = next((r for r in resp.get("quests") or [] if r.get("id") == qid), None)
        if q is None:
            continue
        steps = [
            {
                "id": s.get("id"),
                "title": s.get("title"),
                "description": s.get("description"),
                "progress_current": s.get("progress_current"),
                "progress_total": s.get("progress_total"),
                **({"done": s.get("done")} if all_steps else {}),
            }
            for s in q.get("steps") or []
            if all_steps or not s.get("done")
        ]
        quests_out.append(
            {
                "id": q.get("id"),
                "title": q.get("title"),
                "description": q.get("description"),
                "status": q.get("status"),
                "significance": q.get("significance"),
                "pinned": q.get("pinned"),
                "overlay_hidden": q.get("overlay_hidden"),
                "deadline_at": q.get("deadline_at"),
                "questline_id": q.get("questline_id"),
                "questline_title": q.get("questline_title"),
                "steps": steps,
                "attachments": _attachments_brief("quest", int(qid)),
            }
        )
    return {"quests": quests_out, "count": len(quests_out)}


@server.tool(
    description=(
        "List quests (compact summaries, no step bodies). "
        "Optional filters: status, questline_id, pinned. "
        "Includes attachment metadata (not file bytes) so you can judge "
        "relevance from filename/comment before calling get_attachment."
    )
)
def list_quests(
    status: str | None = None,
    questline_id: int | None = None,
    pinned: bool | None = None,
) -> list[dict[str, Any]]:
    query: dict[str, Any] = {}
    if status:
        query["status"] = status
    if pinned is not None:
        query["pinned"] = str(pinned).lower()
    rows = _api_get("/api/quests", query) or []
    if questline_id is not None:
        rows = [q for q in rows if q.get("questline_id") == questline_id]
    out = []
    for q in rows:
        row = _quest_summary(q)
        qid = q.get("id")
        if qid is not None:
            row["attachments"] = _attachments_brief("quest", int(qid))
        out.append(row)
    return out


@server.tool(
    description=(
        "Only the live work: quests with status active/expired, each with just "
        "its undone steps (done ones dropped). No completed/failed/archived "
        "quests, no finished steps — use this instead of list_quests when you "
        "just need 'what's actually pending', without wading through history. "
        "Optional questline filter (id or name, e.g. questline='Сайт Рефкул'); "
        "omit for everything pending across all questlines. Complements "
        "get_inactive_context — together they cover every quest, every step."
    )
)
def get_active_context(questline: str | int | None = None) -> dict[str, Any]:
    return _quests_by_status_context(("active", "expired"), questline, all_steps=False)


@server.tool(
    description=(
        "The history: quests with status completed/failed/archived, each with "
        "all of its steps (done and not, each flagged `done`) — the complement "
        "of get_active_context. Use it for 'what did we already do' or 'what's "
        "in this questline besides the live stuff', instead of list_quests when "
        "you also want step bodies. Optional questline filter (id or name). "
        "get_active_context ∪ get_inactive_context ≈ every quest in scope, full "
        "steps — the same ground get_context used to cover before it was split "
        "into these two status-scoped tools plus get_note_context for notes."
    )
)
def get_inactive_context(questline: str | int | None = None) -> dict[str, Any]:
    return _quests_by_status_context(
        ("completed", "failed", "archived"), questline, all_steps=True
    )


@server.tool(
    description=(
        "Relations map around one quest / note / questline, as a skeleton: "
        "nodes (key like quest:5, type, id, title, status, depth) and edges "
        "(from, to, kind: structure = belongs to, reference = cited in text, "
        "dependency = order/progress/template). No bodies, so it stays small: "
        "use the ids with get_base_context / get_note_context for details. "
        "Titles of PRIVATE notes are shown (contents never are). depth 1-4, "
        "default 2; `truncated` means the map hit its size cap."
    )
)
def get_graph(entity: str, entity_id: int, depth: int = 2) -> dict[str, Any]:
    if entity not in ("quest", "note", "questline"):
        raise ValueError("entity must be quest, note or questline")
    g = _api_get("/api/graph", {entity: entity_id, "depth": max(1, min(4, depth))}) or {}
    nodes = [
        {
            k: n[k]
            for k in ("key", "type", "id", "title", "status", "depth")
            if n.get(k) not in (None, "")
        }
        for n in g.get("nodes") or []
    ]
    edges = [
        {"from": e["from"], "to": e["to"], "kind": e["kind"]} for e in g.get("edges") or []
    ]
    return {
        "root": g.get("root"),
        "depth": g.get("depth"),
        "truncated": bool(g.get("truncated")),
        "nodes": nodes,
        "edges": edges,
    }


@server.tool(
    description=(
        "Runners: devices that execute step checks / template commands (run_on / "
        "emit_run_on = the runner's label; a comma-separated list is tried in order). Returns label, online (seen within ~70s), "
        "last_seen_at, hostname, version, registered/revoked and queued/running job "
        "counts. Registering a runner (one-time token) is done by the user: "
        "`quests-server runner add <label>` or the Runners tab."
    )
)
def list_runners() -> list[dict[str, Any]]:
    return _api_get("/api/runners") or []


def _trim(text: str | None, limit: int = 1500) -> str:
    text = text or ""
    return text if len(text) <= limit else "…" + text[-limit:]


@server.tool(
    description=(
        "Runner jobs, newest first: state (queued/assigned/running/success/fail/timeout/"
        "lost/skipped/abandoned/canceled), kind (poll/once/emit_pool), runner_label, "
        "quest_id/step_id/template_id, attempt_no, timings, exit_code, progress, reason "
        "and trimmed stdout/stderr/log_tail. `skipped` = the device never came online in "
        "time, `abandoned` = it vanished mid-run (outcome unknown), `lost` = lease "
        "lapsed, a late result may still arrive. Filter by label and/or state "
        "(comma-separated); limit ≤ 100."
    )
)
def list_runner_jobs(
    label: str | None = None, state: str | None = None, limit: int = 30
) -> list[dict[str, Any]]:
    rows = _api_get(
        "/api/runner-jobs",
        {"label": label, "state": state, "limit": max(1, min(100, limit))},
    ) or []
    keep = (
        "id", "state", "kind", "runner_label", "quest_id", "step_id", "template_id", "title",
        "attempt_no", "created_at", "assigned_at", "started_at", "finished_at", "valid_until",
        "exit_code", "timed_out", "progress_current", "progress_total", "reason",
    )
    out = []
    for j in rows:
        item = {k: j.get(k) for k in keep if j.get(k) not in (None, "")}
        for k in ("stdout", "stderr", "log_tail"):
            if j.get(k):
                item[k] = _trim(j[k])
        out.append(item)
    return out


@server.tool(
    description=(
        "Cancel a runner job: a queued one is dropped at once, a running one is told to "
        "stop on its next heartbeat. A cancelled one-shot check parks its step (it is "
        "not queued again by itself)."
    )
)
def cancel_runner_job(job_id: int) -> dict[str, Any]:
    _api("POST", f"/api/runner-jobs/{int(job_id)}/cancel")
    return {"cancelled": int(job_id)}


@server.tool(description="List all questlines (id, title, category, color, …).")
def list_questlines() -> list[dict[str, Any]]:
    return _api_get("/api/questlines") or []


@server.tool(
    description=(
        "List tags — flat work-surface labels (frontend, api, mcp…). "
        "Optional q filters by slug/label substring. Always call this before "
        "create_tag; prefer attaching an existing tag."
    )
)
def list_tags(q: str | None = None) -> list[dict[str, Any]]:
    query: dict[str, Any] = {}
    if q:
        query["q"] = q
    return _api_get("/api/tags", query or None) or []


@server.tool(
    description=(
        "Create a tag (POST /api/tags). Prefer list_tags first and reuse. "
        "slug is the canon (normalized lower/hyphen); label is the sidebar bookmark "
        "text — keep ≤6 chars when possible (frontend→front, infrastructure→infra); "
        "slug may stay longer. label defaults to slug; color optional hex (otherwise "
        "random). Duplicate slug → existing tag returned."
    )
)
def create_tag(
    slug: str | None = None,
    label: str | None = None,
    color: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if slug is not None:
        body["slug"] = slug
    if label is not None:
        body["label"] = label
    if color is not None:
        body["color"] = color
    if not body.get("slug") and not body.get("label"):
        raise ValueError("slug or label required")
    return _api_create_tag(body)


def _api_create_tag(body: dict[str, Any]) -> dict[str, Any]:
    """POST /api/tags; 409 body is the existing tag (reuse)."""
    url = f"{API_BASE}/api/tags"
    headers = {"Accept": "application/json", "Content-Type": "application/json"}
    token = (os.environ.get("QUESTS_API_TOKEN") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(
        url, data=json.dumps(body).encode("utf-8"), headers=headers, method="POST"
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
            return json.loads(raw.decode("utf-8")) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace")
        if e.code == 409:
            try:
                return json.loads(raw)
            except json.JSONDecodeError:
                pass
        detail = raw
        try:
            parsed = json.loads(raw)
            detail = parsed.get("detail", detail)
        except json.JSONDecodeError:
            pass
        raise RuntimeError(f"API {e.code}: {detail}") from e


@server.tool(
    description=(
        "Replace tags on a quest (PUT /api/quests/{id}/tags). Pass tag_ids "
        "and/or tags (slugs/labels); max 5. Empty list clears. Prefer list_tags "
        "to resolve ids first."
    )
)
def set_quest_tags(
    quest_id: int,
    tag_ids: list[int] | None = None,
    tags: list[str] | None = None,
) -> list[dict[str, Any]]:
    body: dict[str, Any] = {}
    if tag_ids is not None:
        body["tag_ids"] = [int(x) for x in tag_ids]
    if tags is not None:
        body["tags"] = list(tags)
    if "tag_ids" not in body and "tags" not in body:
        body["tag_ids"] = []
    return _api("PUT", f"/api/quests/{int(quest_id)}/tags", body=body) or []


@server.tool(
    description=(
        "Replace tags on a template (PUT /api/templates/{id}/tags). Copied onto "
        "materialized quests. Same shape as set_quest_tags."
    )
)
def set_template_tags(
    template_id: int,
    tag_ids: list[int] | None = None,
    tags: list[str] | None = None,
) -> list[dict[str, Any]]:
    body: dict[str, Any] = {}
    if tag_ids is not None:
        body["tag_ids"] = [int(x) for x in tag_ids]
    if tags is not None:
        body["tags"] = list(tags)
    if "tag_ids" not in body and "tags" not in body:
        body["tag_ids"] = []
    return _api("PUT", f"/api/templates/{int(template_id)}/tags", body=body) or []


@server.tool(
    description=(
        "List quest templates — recurring definitions the scheduler materializes "
        "into quest instances daily/weekly (GET /api/templates). enabled filters "
        "on/off; omit for both."
    )
)
def list_templates(enabled: bool | None = None) -> list[dict[str, Any]]:
    query: dict[str, Any] = {}
    if enabled is not None:
        query["enabled"] = "1" if enabled else "0"
    return _api_get("/api/templates", query or None) or []


@server.tool(description="Get one quest template by id, including its steps (GET /api/templates/{id}).")
def get_template(template_id: int) -> dict[str, Any]:
    return _api_get(f"/api/templates/{template_id}")


@server.tool(
    description=(
        "Create a recurring quest template (POST /api/templates). "
        "freq: daily|weekly (weekdays only matters for weekly: comma-separated "
        "0=Mon..6=Sun, default all days). "
        "emit_mode: fixed (materializes unconditionally at the period's first "
        "scheduler tick — deadline_time/duration_seconds only set the resulting "
        "quest's own due time, they do NOT delay creation) or surprise (a cached "
        "per-day chance roll landing at a random moment inside "
        "emit_window_start..emit_window_end). "
        "emit_run_on=<runner label> runs emit_pool_command on a device (a "
        "quests-runner registered under that label); it is required whenever a command is set (the server runs "
        "no commands itself). emit_pool_command is independent of emit_mode and optional: a shell "
        "one-liner, OR — if it starts with a shebang line (#!/usr/bin/env python3 "
        "etc.) — a full inline script. quests writes that text to a temp file and "
        "executes it directly (its own shebang picks the interpreter), so the "
        "script lives in this DB row, not a path on whichever host runs the "
        "scheduler — nothing to hand-deploy. Either way stdout must be ONE quest "
        "as a JSON object shaped like create_quest's body: title?, description?, "
        "significance?, pinned?, automated?, deadline_at? (absolute) + "
        "duration_seconds?, questline?/questline_id?, category?/category_id?, "
        "tags? (slugs or ids), steps? [{title, description?, progress_total?, "
        "check_command?, check_interval_seconds?, run_mode?, wait_previous?, run_on?}]. What "
        "it sets overrides the template's own fields, the rest comes from the "
        "template (and the template's steps are the fallback when it prints none). "
        "Fields outside this list are ignored and named in the attempt log. null, {} "
        "or nothing printed means no quest this period — not an error. There is no "
        "repeat protection: if the command keeps printing the same thing (an unread "
        "mail), the quest keeps coming back, by design. emit_limits (JSON object, "
        "keys max_steps=30 / max_title=200 / max_description=20000 / max_command=2000) "
        "overrides the size limits for this template. (The old format, a JSON array "
        "of {title, description?, quest_description?} items, is still accepted for now "
        "and becomes the steps of one quest.) If the command fails or prints an "
        "invalid quest (non-zero exit, timeout, bad JSON, limits, unknown "
        "questline/tag, bad significance), it is retried with growing pauses "
        "(1/5/15 min, 4 attempts); after that the *next* materialize creates a quest "
        "with status=failed whose description holds the last attempt's trace; every "
        "attempt is in list_emit_attempts. Secrets: shared by every script go in the "
        "*server's* root .env; a secret specific to this template (or its "
        "questline/quests/steps) — use set_secret (owner 'template=N'), never "
        "hardcode either kind in the script — unless the user explicitly asks for "
        "hardcoded placeholder/mock values for a one-off manual test. "
        "category/questline accept an id or a name/substring, resolved the same "
        "way as create_quest's. "
        "steps: same shape as create_quest's `steps` — the template's own "
        "steps, used when there is no emit_pool_command or the quest it prints has "
        "no steps."
    )
)
def create_template(
    title: str,
    description: str | None = None,
    freq: str = "daily",
    weekdays: str | None = None,
    timezone: str | None = None,
    emit_mode: str = "fixed",
    emit_chance: float | None = None,
    emit_window_start: str | None = None,
    emit_window_end: str | None = None,
    emit_pool_command: str | None = None,
    emit_run_on: str | None = None,
    emit_limits: dict[str, int] | None = None,
    deadline_time: str | None = None,
    duration_seconds: int | None = None,
    category: str | int | None = None,
    questline: str | int | None = None,
    significance: str | None = None,
    pinned: bool | None = None,
    overlay_hidden: bool | None = None,
    enabled: bool | None = None,
    automated: bool | None = None,
    steps: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {"title": title, "freq": freq, "emit_mode": emit_mode}
    if description is not None:
        body["description"] = description
    if weekdays is not None:
        body["weekdays"] = weekdays
    if timezone is not None:
        body["timezone"] = timezone
    if emit_chance is not None:
        body["emit_chance"] = float(emit_chance)
    if emit_window_start is not None:
        body["emit_window_start"] = emit_window_start
    if emit_window_end is not None:
        body["emit_window_end"] = emit_window_end
    if emit_pool_command is not None:
        body["emit_pool_command"] = emit_pool_command
    if emit_run_on is not None:
        body["emit_run_on"] = emit_run_on
    if emit_limits is not None:
        body["emit_limits"] = emit_limits
    if deadline_time is not None:
        body["deadline_time"] = deadline_time
    if duration_seconds is not None:
        body["duration_seconds"] = int(duration_seconds)
    cat_id = _resolve_category_id(category)
    if cat_id is not None:
        body["category_id"] = cat_id
    line_id = _resolve_questline_id(questline)
    if line_id is not None:
        body["questline_id"] = line_id
    if significance is not None:
        body["significance"] = _check_significance(significance)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if overlay_hidden is not None:
        body["overlay_hidden"] = bool(overlay_hidden)
    if enabled is not None:
        body["enabled"] = bool(enabled)
    if automated is not None:
        body["automated"] = bool(automated)
    if steps is not None:
        body["steps"] = [_step_body(s) for s in steps]
    return _api("POST", "/api/templates", body=body)


@server.tool(
    description=(
        "Update a quest template (PATCH /api/templates/{id}). Only pass fields "
        "to change — same field semantics as create_template. "
        "enabled=true (even re-saving an already-enabled template with some "
        "other field changed) triggers an immediate materialize attempt for the "
        "current period — this is how you fire a fixed-mode or pool template on "
        "demand instead of waiting for the scheduler's own ~15s tick."
    )
)
def update_template(
    template_id: int,
    title: str | None = None,
    description: str | None = None,
    freq: str | None = None,
    weekdays: str | None = None,
    timezone: str | None = None,
    emit_mode: str | None = None,
    emit_chance: float | None = None,
    emit_window_start: str | None = None,
    emit_window_end: str | None = None,
    emit_pool_command: str | None = None,
    emit_run_on: str | None = None,
    emit_limits: dict[str, int] | None = None,
    deadline_time: str | None = None,
    duration_seconds: int | None = None,
    category: str | int | None = None,
    questline: str | int | None = None,
    significance: str | None = None,
    pinned: bool | None = None,
    overlay_hidden: bool | None = None,
    enabled: bool | None = None,
    automated: bool | None = None,
    steps: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if title is not None:
        body["title"] = title
    if description is not None:
        body["description"] = description
    if freq is not None:
        body["freq"] = freq
    if weekdays is not None:
        body["weekdays"] = weekdays
    if timezone is not None:
        body["timezone"] = timezone
    if emit_mode is not None:
        body["emit_mode"] = emit_mode
    if emit_chance is not None:
        body["emit_chance"] = float(emit_chance)
    if emit_window_start is not None:
        body["emit_window_start"] = emit_window_start
    if emit_window_end is not None:
        body["emit_window_end"] = emit_window_end
    if emit_pool_command is not None:
        body["emit_pool_command"] = emit_pool_command
    if emit_run_on is not None:
        body["emit_run_on"] = emit_run_on
    if emit_limits is not None:
        body["emit_limits"] = emit_limits
    if deadline_time is not None:
        body["deadline_time"] = deadline_time
    if duration_seconds is not None:
        body["duration_seconds"] = int(duration_seconds)
    if category is not None:
        cat_id = _resolve_category_id(category)
        if cat_id is not None:
            body["category_id"] = cat_id
    if questline is not None:
        line_id = _resolve_questline_id(questline)
        if line_id is not None:
            body["questline_id"] = line_id
    if significance is not None:
        body["significance"] = _check_significance(significance)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if overlay_hidden is not None:
        body["overlay_hidden"] = bool(overlay_hidden)
    if enabled is not None:
        body["enabled"] = bool(enabled)
    if automated is not None:
        body["automated"] = bool(automated)
    if steps is not None:
        body["steps"] = [_step_body(s) for s in steps]
    if not body:
        raise ValueError("provide at least one field to update")
    return _api("PATCH", f"/api/templates/{template_id}", body=body)


@server.tool(
    description=(
        "Delete a quest template (DELETE /api/templates/{id}). Does not touch "
        "quests it already materialized."
    )
)
def delete_template(template_id: int) -> dict[str, Any]:
    _api("DELETE", f"/api/templates/{template_id}")
    return {"deleted": template_id}


@server.tool(
    description=(
        "List the secret *names* set for a template's emit_pool_command "
        "(GET /api/templates/{id}/secrets) — never the values. Use this "
        "instead of secrets shared by every script via the server's root "
        ".env (see create_template's note on that) when a script needs a "
        "credential specific to just this one template — e.g. "
        "template id=5 'Почта' needing MAIL_HOST/MAIL_USER/MAIL_PASSWORD "
        "without also handing that mailbox's password to every other "
        "script that reads the process env. Values are stored as plain "
        "text (the server itself is the trust boundary — the script runs "
        "there anyway) but are injected into the child process env only "
        "at the moment the scheduler actually executes the command; no "
        "read endpoint or tool (including get_template/list_templates) "
        "ever returns them."
    )
)
def list_template_secrets(template_id: int) -> list[str]:
    return (_api_get(f"/api/templates/{template_id}/secrets") or {}).get("keys", [])


def _secret_key_path(template_id: int, key: str) -> str:
    return f"/api/templates/{template_id}/secrets/{urllib.parse.quote(key, safe='')}"


@server.tool(
    description=(
        "Set (or overwrite) one secret for a template's emit_pool_command "
        "(PUT /api/templates/{id}/secrets/{key}) — write-only, the value is "
        "never echoed back or readable again through any tool; use "
        "list_template_secrets to see what's set. key must look like an "
        "env var name ([A-Za-z_][A-Za-z0-9_]*) — it becomes exactly that "
        "env var in the script's process, nothing more to reference it by."
    )
)
def set_template_secret(template_id: int, key: str, value: str) -> dict[str, Any]:
    _api("PUT", _secret_key_path(template_id, key), body={"value": value})
    return {"template_id": template_id, "key": key, "set": True}


_SECRET_OWNERS = {
    "questline": "questlines",
    "template": "templates",
    "quest": "quests",
    "step": "steps",
    "note": "notes",
}


def _secret_owner_path(owner: str) -> str:
    kind, _, raw = (owner or "").strip().partition("=")
    plural = _SECRET_OWNERS.get(kind.strip().lower())
    if plural is None or not raw.strip().isdigit():
        raise ValueError(
            f"owner must look like 'quest=12' with kind one of {sorted(_SECRET_OWNERS)} (got {owner!r})"
        )
    return f"/api/{plural}/{int(raw)}/secrets"


@server.tool(
    description=(
        "Secret *names* for an owner, never values (GET /api/{kind}s/{id}/secrets). "
        "owner is 'questline=N' | 'template=N' | 'quest=N' | 'step=N' | 'note=N' (a note's secrets are its own, not inherited). Returns "
        "{keys: set directly on the owner, inherited: [{key, from, id}] coming from "
        "a more general owner}. Secrets are inherited step > quest > template > "
        "questline: a step's check_command and a template's emit_pool_command get them "
        "as environment variables (masked as *** in whatever they print)."
    )
)
def list_secrets(owner: str) -> dict[str, Any]:
    return _api("GET", _secret_owner_path(owner))


@server.tool(
    description=(
        "Set (or overwrite) one secret on an owner (PUT /api/{kind}s/{id}/secrets/{key}); "
        "owner is 'questline=N' | 'template=N' | 'quest=N' | 'step=N' | 'note=N' (a note's secrets are its own, not inherited). Write-only: the "
        "value is never readable again. key must be an env var name "
        "([A-Za-z_][A-Za-z0-9_]*). Everything below the owner inherits it (questline → "
        "its quests and templates → their steps); a more specific owner overrides a "
        "same-named secret. Anyone who can write a step command can print these values "
        "(masking only catches the literal value), so put a secret on the narrowest "
        "owner that needs it."
    )
)
def set_secret(owner: str, key: str, value: str) -> dict[str, Any]:
    _api("PUT", f"{_secret_owner_path(owner)}/{urllib.parse.quote(key, safe='')}", body={"value": value})
    return {"owner": owner, "key": key, "set": True}


@server.tool(
    description="Remove one secret from an owner ('questline=N' | 'template=N' | 'quest=N' | 'step=N' | 'note=N')."
)
def delete_secret(owner: str, key: str) -> dict[str, Any]:
    _api("DELETE", f"{_secret_owner_path(owner)}/{urllib.parse.quote(key, safe='')}")
    return {"owner": owner, "key": key, "deleted": True}


@server.tool(
    description="Remove one template secret (DELETE /api/templates/{id}/secrets/{key})."
)
def delete_template_secret(template_id: int, key: str) -> dict[str, Any]:
    _api("DELETE", _secret_key_path(template_id, key))
    return {"template_id": template_id, "key": key, "deleted": True}


@server.tool(
    description=(
        "Execution log of a template's emit_pool_command, newest first "
        "(GET /api/templates/{id}/emit-attempts). One row per run: period_key, "
        "attempt number within the period, status (ok | error | timeout | "
        "bad_json | bad_spec), duration_ms, step count of the printed quest, and a "
        "message (the verdict like 'miss: the command printed no quest' or "
        "'quest «…», 3 step(s); ignored fields: …', plus stderr/trace). "
        "Template secret values are masked as *** in messages. Use it to find "
        "out why a template shows emit_pool_last_outcome=miss/error. "
        "limit defaults to 50 (max 200); the server keeps the last 200 per template."
    )
)
def list_emit_attempts(template_id: int, limit: int | None = None) -> dict[str, Any]:
    return _api(
        "GET",
        f"/api/templates/{template_id}/emit-attempts",
        query={"limit": limit},
    )


# Mirrors go/internal/schedule/materialize.go: emitPoolTimeout (20s) and the
# shebang-body-vs-sh-one-liner dispatch of execEmitPoolCommand. Kept in sync by
# hand — this is a dry-run, not the scheduler's own execution path.
_EMIT_POOL_TIMEOUT_SECONDS = 20
_EMIT_POOL_MAX_ATTEMPTS = 3


def _run_emit_pool_command(command: str) -> dict[str, Any]:
    """Execute an emit_pool_command exactly as the Go scheduler would and parse
    its stdout. Returns a structured trace instead of raising — a failing script
    is the normal thing you're debugging here.
    """
    stripped = command.lstrip(" \t\r\n")
    home = os.path.expanduser("~")
    env = os.environ  # main() already loaded ROOT/.env, same source the server uses
    script_path: str | None = None
    try:
        if stripped.startswith("#!"):
            exec_mode = "script"  # shebang body → temp file, its own interpreter
            fd, script_path = tempfile.mkstemp(prefix="quests-emit-pool-")
            with os.fdopen(fd, "w") as fh:
                fh.write(command)
            os.chmod(script_path, 0o700)
            argv = [script_path]
        else:
            exec_mode = "sh -c"
            argv = ["sh", "-c", command]
        try:
            proc = subprocess.run(
                argv,
                cwd=home,
                env=env,
                capture_output=True,
                text=True,
                timeout=_EMIT_POOL_TIMEOUT_SECONDS,
            )
        except subprocess.TimeoutExpired as e:
            return {
                "ok": False,
                "exec_mode": exec_mode,
                "error": f"timed out after {_EMIT_POOL_TIMEOUT_SECONDS}s",
                "stdout": _clip(e.stdout or ""),
                "stderr": _clip(e.stderr or ""),
            }
    finally:
        if script_path:
            try:
                os.remove(script_path)
            except OSError:
                pass

    trace = {
        "exec_mode": exec_mode,
        "exit_code": proc.returncode,
        "stdout": _clip(proc.stdout),
        "stderr": _clip(proc.stderr),
    }
    if proc.returncode != 0:
        # Same as the scheduler: a non-zero exit is a failure regardless of
        # stdout — after _EMIT_POOL_MAX_ATTEMPTS it materializes a failed quest.
        trace["ok"] = False
        trace["error"] = f"non-zero exit ({proc.returncode})"
        return trace
    text = proc.stdout.strip()
    try:
        raw = json.loads(text) if text else None
    except json.JSONDecodeError as e:
        trace["ok"] = False
        trace["error"] = f"invalid JSON on stdout: {e}"
        return trace
    known = {
        "title", "description", "significance", "pinned", "deadline_at",
        "duration_seconds", "questline", "questline_id", "category", "category_id",
        "tags", "tag_ids", "automated", "steps",
    }
    if raw is None or raw == {} or raw == []:
        trace["ok"] = True
        trace["quest"] = None
        trace["note"] = "no quest this period (null / {} / nothing printed)"
    elif isinstance(raw, dict):
        steps = raw.get("steps") if isinstance(raw.get("steps"), list) else []
        trace["ok"] = True
        trace["quest"] = raw
        trace["steps_count"] = len(steps)
        ignored = sorted(k for k in raw if k not in known)
        if ignored:
            trace["ignored_fields"] = ignored
    elif isinstance(raw, list):
        titles = [str(e.get("title") or "").strip() for e in raw if isinstance(e, dict)]
        trace["ok"] = True
        trace["legacy_items"] = [t for t in titles if t]
        trace["note"] = (
            "old-style item array: the server still accepts it and turns the items "
            "into the steps of one quest, but print one quest object instead"
        )
    else:
        trace["ok"] = False
        trace["error"] = "stdout JSON must be a quest object (or null)"
    return trace


def _clip(s: str, n: int = 4000) -> str:
    s = (s or "").strip()
    return s if len(s) <= n else s[:n] + "…"


@server.tool(
    description=(
        "Dry-run an emit_pool_command and show the quest it would print — the "
        "missing half of authoring an emitting template through MCP (create_template/"
        "update_template can already store the command). Runs it exactly as the "
        "scheduler does: a value starting with '#!' is written to a temp file "
        "and executed by its own shebang, anything else runs via `sh -c`; cwd is "
        "$HOME, env is the server process env (root .env already loaded), 20s "
        "timeout. Pass either `command` (the script/one-liner text you're "
        "drafting) or `template_id` (dry-run its stored emit_pool_command). "
        "Returns ok plus the quest object (or null = no quest this period), its "
        "step count and any ignored (unknown) fields, or on failure the exit code "
        "and stdout/stderr trace — the same signal the scheduler would act on. Does "
        "NOT validate against the template's limits, persist a roll, or "
        "materialize a quest; it just shows what the command emits right now. "
        "CAUTION, two ways a green result here can still mislead: (1) this "
        "tool runs the command for real on whatever host the MCP server "
        "itself is on — not inside the API server's own environment (its "
        "own container/host, its own env file). A command that shells out to "
        "host-only tools, or reads env vars only the API server's env file "
        "sets, can pass here and still fail for real — confirmed twice: once "
        "against pacman/checkupdates/vercmp missing in the API container, "
        "once against a script needing env vars present in the MCP host's "
        ".env but absent from the API container's own env file — the same "
        "gap applies to anything set via set_template_secret, which this "
        "tool never sees at all (those are injected only by the real "
        "scheduler, inside the API server's own process). (2) it does "
        "NOT sandbox the command — any side effect the command performs for "
        "real (network writes, marking something read/consumed upstream, "
        "local files) happens for real, exactly as if the scheduler ran it; "
        "only Quests' own bookkeeping (the roll row, the quest) is skipped. "
        "For a script with real side effects, dry-run it against a copy with "
        "a throwaway target (a test marker path, a sandbox account, etc.) "
        "instead of the one it'll actually use."
    )
)
def dry_run_emit_pool(
    command: str | None = None,
    template_id: int | None = None,
) -> dict[str, Any]:
    chosen = [v for v in (command, template_id) if v is not None]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: command, template_id")
    if template_id is not None:
        tmpl = _api_get(f"/api/templates/{template_id}")
        cmd = (tmpl or {}).get("emit_pool_command")
        if not cmd or not str(cmd).strip():
            raise ValueError(f"template {template_id} has no emit_pool_command set")
        source: dict[str, Any] = {"source": f"template={template_id}"}
        cmd = str(cmd)
    else:
        cmd = command
        source = {"source": "inline"}
    result = _run_emit_pool_command(cmd)
    return {**source, **result}


def _note_children(rows: list[dict[str, Any]]) -> dict[Any, list[dict[str, Any]]]:
    kids: dict[Any, list[dict[str, Any]]] = {}
    for n in rows:
        kids.setdefault(n.get("parent_id"), []).append(n)
    return kids


def _note_descendants(rows: list[dict[str, Any]], root_id: int) -> list[dict[str, Any]]:
    """All notes under root_id (not root itself), parent-before-child order."""
    kids = _note_children(rows)
    out: list[dict[str, Any]] = []
    stack = list(reversed(kids.get(root_id, [])))
    seen: set[Any] = set()
    while stack:
        n = stack.pop()
        if n["id"] in seen:
            continue
        seen.add(n["id"])
        out.append(n)
        stack.extend(reversed(kids.get(n["id"], [])))
    return out


def _note_path(by_id: dict[Any, dict[str, Any]], n: dict[str, Any]) -> str:
    parts = [n.get("title") or f"note={n['id']}"]
    pid = n.get("parent_id")
    seen = {n["id"]}
    while pid is not None and pid in by_id and pid not in seen:
        seen.add(pid)
        parts.append(by_id[pid].get("title") or f"note={pid}")
        pid = by_id[pid].get("parent_id")
    return " / ".join(reversed(parts))


def _note_brief(n: dict[str, Any]) -> dict[str, Any]:
    out = {k: v for k, v in n.items() if k != "description"}
    out["description_len"] = len(n.get("description") or "")
    return out


def _category_hint(n: dict[str, Any]) -> str:
    """First non-empty description line = what belongs in this category."""
    if n.get("is_private"):
        return ""
    for line in (n.get("description") or "").splitlines():
        line = line.strip().lstrip("#>*- ").strip()
        if line:
            return line[:200]
    return ""


def _category_rows(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_id = {n["id"]: n for n in rows}
    kids = _note_children(rows)
    return [
        {
            "id": n["id"],
            "title": n.get("title"),
            "parent_id": n.get("parent_id"),
            "path": _note_path(by_id, n),
            "children": len(kids.get(n["id"], [])),
            "hint": _category_hint(n),
        }
        for n in rows
        if n.get("is_category")
    ]


@server.tool(
    description=(
        "List knowledge notes (markdown pages, not quests). Optional parent_id "
        "filters children of one note; omit for the whole vault. "
        "category_id narrows to everything under one CATEGORY note (all "
        "descendants, not the category itself) — get ids from "
        "list_note_categories. brief=true drops each description (keeps "
        "description_len) so the whole vault fits in context — use it to "
        "scan titles/structure, then get_note_context for the ones you need. "
        "Without brief the full vault dump is huge. "
        "Link from quests with note=N in the description. Each row carries "
        "is_readme/is_private/is_category. A PRIVATE note's description is "
        "replaced by a placeholder in this list too — same rule as "
        "get_note_context, don't work around it."
        "sort='popular' orders by reading history (recently read first, decayed over ~30 days, "
        "new notes get a small head start) and adds a `popularity` block per note with the reason; "
        "it is a hint for what to look at first, not a verdict — relevance to your task wins. "
        "Each row has `outdated` / `outdated_reason` (the OUTDATED flag) and `has_check`. "
        "flags narrows by note flags, comma-separated, a minus means \"without\": "
        "flags=\"outdated\" lists the stale notes, flags=\"-outdated,-private\" skips them "
        "(known: outdated, readme, private, category, pinned, check)."
    )
)
def list_notes(
    parent_id: int | None = None,
    category_id: int | None = None,
    brief: bool = False,
    sort: str | None = None,
    flags: str = "",
) -> list[dict[str, Any]]:
    query: dict[str, Any] = {}
    if parent_id is not None:
        query["parent_id"] = parent_id
    if sort == "popular":
        query["sort"] = "popular"
    elif sort not in (None, "", "tree"):
        raise ValueError("sort must be 'popular' or omitted (tree order)")
    rows = _api_get("/api/notes", query or None) or []
    if category_id is not None:
        everything = _api_get("/api/notes") or []
        root = next((n for n in everything if n["id"] == int(category_id)), None)
        if root is None:
            raise ValueError(f"note={category_id} not found")
        if not root.get("is_category"):
            raise ValueError(
                f"note={category_id} is not a CATEGORY note (see list_note_categories)"
            )
        under = {n["id"] for n in _note_descendants(everything, int(category_id))}
        rows = [n for n in rows if n["id"] in under]
    rows = _filter_note_flags(rows, flags)
    return [_note_brief(n) for n in rows] if brief else rows


_NOTE_FLAG_FIELDS = {
    "outdated": "outdated",
    "stale": "outdated",
    "readme": "is_readme",
    "private": "is_private",
    "category": "is_category",
    "pinned": "pinned",
    "pin": "pinned",
    "check": "has_check",
}


def _filter_note_flags(rows: list[dict[str, Any]], flags: str) -> list[dict[str, Any]]:
    """Keep the notes that have every flag of `flags` and none of the `-flag`s."""
    want: list[tuple[str, bool]] = []
    for part in str(flags or "").split(","):
        part = part.strip()
        if not part:
            continue
        neg = part.startswith("-")
        field = _NOTE_FLAG_FIELDS.get(part.lstrip("-").lower())
        if field is None:
            raise ValueError(f"unknown flag {part!r}; known: {', '.join(sorted(_NOTE_FLAG_FIELDS))}")
        want.append((field, neg))
    return [n for n in rows if all(bool(n.get(f)) != neg for f, neg in want)]


@server.tool(
    description=(
        "Give a note a freshness check, or remove it (empty command). The command runs like a step's "
        "check: on a runner (run_on = runner label, required; a,b = first online, then the next), with the "
        "note's own secrets in its environment, every interval_seconds (default 3600, minimum 15). It may be "
        "several lines, or a script with its own interpreter (starts with #!/usr/bin/env python3 …). It must "
        "print 0 or 1 on its last line (1 = the note is out of date) or JSON {\"outdated\": 0|1, "
        "\"reason\": \"…\"}. A broken run (bad output, failure, empty output) is recorded as an error and "
        "leaves the verdict alone. The command keeps its own baseline — the platform stores none: the "
        "reference lives in the command itself (e.g. REF=<commit> at its top) and is re-pinned by hand when "
        "the note is rewritten. Typical form for a note about files in a public GitHub repo: curl "
        "https://api.github.com/repos/OWNER/REPO/compare/$REF...main and look at .files[].filename with jq "
        "(see docs/note-check.md in the Quests repo). $QUESTS_NOTE_ID is set. Only workspace owners may "
        "set a command. After you rewrite a note, re-pin REF with set_note_check and call check_note_now."
    )
)
def set_note_check(
    note_id: int,
    command: str | None = None,
    interval_seconds: int | None = None,
    run_on: str | None = None,
) -> dict[str, Any]:
    return _api(
        "PUT",
        f"/api/notes/{int(note_id)}/check",
        body={"command": command, "interval_seconds": interval_seconds, "run_on": run_on},
    )


@server.tool(
    description=(
        "Run a note's freshness check at the next scheduler tick (within ~15 s) instead of waiting for "
        "its interval. The verdict shows in get_note_context / list_notes (`outdated`, `outdated_reason`, "
        "`check.error`)."
    )
)
def check_note_now(note_id: int) -> dict[str, Any]:
    return _api("POST", f"/api/notes/{int(note_id)}/check/run")


@server.tool(
    description=(
        "Set or clear the OUTDATED flag of a note by hand: the note's content no longer matches reality "
        "(outdated=true, with a short reason saying what drifted) or it has been rewritten and is fresh "
        "again (outdated=false). It is the same flag a freshness check writes, shown as a red clock-with-"
        "exclamation icon, filterable with flag:outdated, and such notes rank lower in search_notes. "
        "If the note has a check command, its next run gives its own verdict over yours — to keep a hand-"
        "set flag, leave the check alone or remove it. Use it when you notice a note is wrong; do not "
        "silently rewrite and forget: mark it, or fix it and clear the flag."
    )
)
def set_note_outdated(note_id: int, outdated: bool = True, reason: str = "") -> dict[str, Any]:
    return _api(
        "PUT",
        f"/api/notes/{int(note_id)}/outdated",
        body={"outdated": bool(outdated), "reason": reason},
    )


@server.tool(
    description=(
        "Tell the vault whether a note helped (useful=true) or misled you (useful=false). Cheap, "
        "and the one signal that is worth more than a read: it moves the note up or down in "
        "search_notes / list_notes(sort='popular') for everyone. Call it after you actually used "
        "(or were misled by) a note, not for every note you glanced at. Returns the note's "
        "popularity block (score, opens, ratings, why)."
    )
)
def rate_note(note_id: int, useful: bool) -> dict[str, Any]:
    return _api("POST", f"/api/notes/{int(note_id)}/feedback", body={"useful": bool(useful)})


@server.tool(
    description=(
        "Search notes like a search engine: ranked by relevance, not by substring. Understands "
        "Russian and English word forms (квесты/квестов), prefixes of the last word, the other "
        "keyboard layout (ghbdtn → привет), small typos in titles, \"exact phrases\" and -excluded words. "
        "Each hit has id, title, score, where it matched (title | body | fuzzy) and a snippet with "
        "the matched words in ⟪ ⟫ — read the snippet to decide whether to open the note with "
        "get_note_context. PRIVATE notes never appear here. Prefer this to list_notes when you "
        "look for what a note says rather than where it sits in the tree. `flags` narrows by note flags, "
        "comma-separated, a minus means \"without\": e.g. flags=\"-outdated\" skips notes marked stale "
        "(known: outdated, readme, category, pinned, check; private never reaches an agent)."
    )
)
def search_notes(q: str, limit: int = 10, flags: str = "") -> dict[str, Any]:
    if not str(q).strip():
        raise ValueError("q is empty")
    params: dict[str, Any] = {"q": q, "limit": max(1, min(int(limit), 50))}
    if flags.strip():
        params["flags"] = flags.strip()
    return _api_get("/api/notes/search", params) or {"hits": []}


@server.tool(
    description=(
        "List only CATEGORY notes (folder-like aggregators of the notes vault): "
        "id, title, path, direct child count, and hint — the first line of the "
        "category's description, i.e. what belongs there. Cheap (a handful of "
        "rows) — call it before create_note to pick a parent_id, and as the "
        "entry point to list_notes(category_id=…)."
    )
)
def list_note_categories() -> list[dict[str, Any]]:
    return _category_rows(_api_get("/api/notes") or [])


def _category_refusal(cats: list[dict[str, Any]]) -> str:
    lines = [
        "create_note refused: no parent_id, but the vault has CATEGORY notes. "
        "Check whether the new note logically fits one of them:"
    ]
    for c in cats:
        hint = f" — {c['hint']}" if c["hint"] else ""
        lines.append(f"  note={c['id']} {c['path']} ({c['children']} children){hint}")
    lines.append(
        "Retry with parent_id=<category id (or a note under it)>; if none "
        "fits, retry with no_category=true."
    )
    return "\n".join(lines)


@server.tool(
    description=(
        "Create a markdown knowledge page (POST /api/notes). Not a quest — no "
        "status, steps, deadline, HUD. parent_id nests it under another note "
        "(vault tree only). Put toolkits (readme/script/config) in description "
        "as markdown/code fences. Cite from quests with note=N. "
        "CATEGORY guard: when the vault has CATEGORY notes and you pass no "
        "parent_id, the call is refused with the category list. Decide whether "
        "the new note logically belongs in one (read the hints; "
        "list_note_categories / list_notes(category_id=…, brief=true) to look "
        "closer) and retry with parent_id=<category or a note under it>; only "
        "if nothing fits retry with no_category=true. Creating a CATEGORY "
        "itself (is_category=true) is exempt."
    )
)
def create_note(
    title: str,
    description: str | None = None,
    parent_id: int | None = None,
    pinned: bool | None = None,
    is_readme: bool | None = None,
    is_private: bool | None = None,
    is_category: bool | None = None,
    sort_order: int | None = None,
    no_category: bool = False,
) -> dict[str, Any]:
    if parent_id is None and not is_category and not no_category:
        cats = _category_rows(_api_get("/api/notes") or [])
        if cats:
            raise ValueError(_category_refusal(cats))
    body: dict[str, Any] = {"title": title}
    if description is not None:
        body["description"] = description
    if parent_id is not None:
        body["parent_id"] = int(parent_id)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if is_readme is not None:
        body["is_readme"] = bool(is_readme)
    if is_private is not None:
        body["is_private"] = bool(is_private)
    if is_category is not None:
        body["is_category"] = bool(is_category)
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    return _api("POST", "/api/notes", body=body)


@server.tool(
    description=(
        "Update a note (PATCH /api/notes/{id}). Only pass fields to change. "
        "description is markdown and replaces the whole body. For partial edits "
        "use insert_text / replace_find instead — they keep the rest of the note. "
        "insert_text: optional insert_after / insert_before (first occurrence) or "
        "insert_at (rune offset 0..len); omit all three to append. "
        "replace_find + replace_with: first match unless replace_all=true. "
        "Missing anchor/find → API 422, not a silent full rewrite. "
        "Do not mix description with insert/replace in one call. "
        "parent_id=null (via clear_parent) detaches to the vault root. "
        "pinned/is_readme/is_private/is_category are independent flags, not "
        "mutually exclusive (see note=82 for what each means)."
    )
)
def update_note(
    note_id: int,
    title: str | None = None,
    description: str | None = None,
    insert_text: str | None = None,
    insert_after: str | None = None,
    insert_before: str | None = None,
    insert_at: int | None = None,
    replace_find: str | None = None,
    replace_with: str | None = None,
    replace_all: bool = False,
    parent_id: int | None = None,
    clear_parent: bool = False,
    pinned: bool | None = None,
    is_readme: bool | None = None,
    is_private: bool | None = None,
    is_category: bool | None = None,
    sort_order: int | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if title is not None:
        body["title"] = title
    ops = sum(
        1
        for flag in (
            description is not None,
            insert_text is not None,
            replace_find is not None,
        )
        if flag
    )
    if ops > 1:
        raise ValueError(
            "provide only one of: description, insert_text, replace_find"
        )
    if description is not None:
        body["description"] = description
    if insert_text is not None:
        op: dict[str, Any] = {"text": insert_text}
        pos = sum(
            1
            for flag in (
                insert_after is not None,
                insert_before is not None,
                insert_at is not None,
            )
            if flag
        )
        if pos > 1:
            raise ValueError("provide at most one of: insert_after, insert_before, insert_at")
        if insert_after is not None:
            op["after"] = insert_after
        if insert_before is not None:
            op["before"] = insert_before
        if insert_at is not None:
            op["at"] = int(insert_at)
        body["description_insert"] = op
    if replace_find is not None:
        if replace_with is None:
            raise ValueError("replace_with is required when replace_find is set")
        body["description_replace"] = {
            "find": replace_find,
            "with": replace_with,
            "all": bool(replace_all),
        }
    if clear_parent:
        body["parent_id"] = None
    elif parent_id is not None:
        body["parent_id"] = int(parent_id)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if is_readme is not None:
        body["is_readme"] = bool(is_readme)
    if is_private is not None:
        body["is_private"] = bool(is_private)
    if is_category is not None:
        body["is_category"] = bool(is_category)
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    if not body:
        raise ValueError("provide at least one field to update")
    return _api("PATCH", f"/api/notes/{note_id}", body=body)


@server.tool(
    description=(
        "Permanently delete a note (DELETE /api/notes/{id}). Also drops its "
        "refs/backlinks. Children are not cascaded — detach or re-parent them "
        "first, or the API will reject the delete if it enforces that."
    )
)
def delete_note(note_id: int) -> dict[str, Any]:
    _api("DELETE", f"/api/notes/{note_id}")
    return {"deleted": note_id}


@server.tool(
    description=(
        "Create a new quest (POST /api/quests). Steps can be passed inline via "
        "`steps`: a list of {title, description?, progress_total?, progress_current?, "
        "sort_order?, check_command?, check_interval_seconds?, wait_previous?, "
        "run_mode?, run_on?} — if omitted, a single default step named after the quest is "
        "created. check_command may be several lines or a script with its own interpreter "
        "(starts with #!/usr/bin/env python3 …). run_mode is poll (stdout: a number, or JSON {progress?, total?, "
        "description?} — total re-sizes the step) or once (exit 0). A step can mirror "
        "another quest: put quest=N in its title/description, run_mode=watch (poll that "
        "also keeps going after the step is closed, active quests only) and check_command "
        "`quests progress N` (prints that quest's closed/total steps). wait_previous "
        "gates the auto-step on the previous step. run_on=<runner label> (or a,b — first online) makes the "
        "check run on another device (a quests-runner registered under that label) "
        "instead of the server; empty = the server. automated=true demotes "
        "created/appeared/started overlay toasts. deadline_at is the moment the quest is due (ISO datetime, UTC — "
        "e.g. '2026-08-20T10:20:00Z'); duration_seconds sizes the urgent/reminder "
        "window that opens that many seconds before deadline_at and triggers the "
        "Telegram/HUD notification (e.g. 7200 for a 2-hour-before reminder). "
        "category and questline accept either a numeric id or a name/substring "
        "(e.g. category='health', questline='Сайт Рефкул') — resolved via "
        "/api/categories and /api/questlines; error if ambiguous. "
        "`description` (and each step's description) is markdown rendered in the "
        "journal — lists, links, inline `code`, emphasis. Title is plain text. "
        "Passing questline "
        "makes the quest inherit that questline's category. "
        "tag_ids: optional list of existing tag ids (max 5); use list_tags / "
        "create_tag first. quiet=true skips overlay toasts. "
        "overlay_hidden=true keeps the quest out of the HUD entirely (pinned=true is the opposite: always shown; "
        "turning one on turns the other off)."
    )
)
def create_quest(
    title: str,
    description: str | None = None,
    status: str | None = None,
    significance: str | None = None,
    pinned: bool | None = None,
    overlay_hidden: bool | None = None,
    sort_order: int | None = None,
    deadline_at: str | None = None,
    duration_seconds: int | None = None,
    category: str | None = None,
    questline: str | None = None,
    steps: list[dict[str, Any]] | None = None,
    tag_ids: list[int] | None = None,
    automated: bool | None = None,
    quiet: bool = True,
) -> dict[str, Any]:
    body: dict[str, Any] = {"title": title}
    if description is not None:
        body["description"] = description
    if status is not None:
        st = str(status).strip().lower()
        if st not in ALLOWED_STATUS:
            raise ValueError(f"bad status {status!r}; expected one of {sorted(ALLOWED_STATUS)}")
        body["status"] = st
    if significance is not None:
        body["significance"] = _check_significance(significance)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if overlay_hidden is not None:
        body["overlay_hidden"] = bool(overlay_hidden)
    if automated is not None:
        body["automated"] = bool(automated)
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    if deadline_at is not None:
        body["deadline_at"] = deadline_at
    if duration_seconds is not None:
        body["duration_seconds"] = int(duration_seconds)
    questline_id = _resolve_questline_id(questline)
    if questline_id is not None:
        body["questline_id"] = questline_id
    else:
        cat_id = _resolve_category_id(category)
        if cat_id is not None:
            body["category_id"] = cat_id
    if steps:
        body["steps"] = [_step_body(s) for s in steps]
    if tag_ids is not None:
        body["tag_ids"] = [int(x) for x in tag_ids]
    q = _api("POST", "/api/quests", query=_tool_query(quiet=quiet), body=body)
    return _quest_mutation_result(q)

@server.tool(
    description=(
        "Ping the user when you're blocked on their input and they may have stepped "
        "away from this conversation — creates a quest the normal (never quiet) way, "
        "so it rides the same overlay fullscreen-toast/HUD path a human-created quest "
        "does and is hard to miss. Not for routine progress updates or anything you "
        "can just say in chat — only when you are genuinely stuck without their answer. "
        "message becomes the quest title; details is an optional longer note."
    )
)
def ping_user(
    message: str,
    details: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {"title": message, "pinned": True, "significance": "legendary"}
    if details is not None:
        body["description"] = details
    q = _api("POST", "/api/quests", query=_tool_query(quiet=False), body=body)
    return _quest_mutation_result(q)


@server.tool(
    description=(
        "Create a new questline (POST /api/questlines) — a themed project/series "
        "that quests can be attached to via update_quest(questline_id=...) or "
        "create_quest(questline=...). category accepts id or name/substring."
    )
)
def create_questline(
    title: str,
    description: str | None = None,
    category: str | None = None,
    color: str | None = None,
    icon: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {"title": title}
    if description is not None:
        body["description"] = description
    if color is not None:
        body["color"] = color
    if icon is not None:
        body["icon"] = icon
    cat_id = _resolve_category_id(category)
    if cat_id is not None:
        body["category_id"] = cat_id
    return _api("POST", "/api/questlines", body=body)


@server.tool(
    description=(
        "Update a questline (PATCH /api/questlines/{id}). Only pass fields to "
        "change. description is markdown. category accepts an id or a "
        "name/substring. icon is the string code (e.g. 'target'), not a custom "
        "uploaded image — see set_icon for that."
    )
)
def update_questline(
    questline_id: int,
    title: str | None = None,
    description: str | None = None,
    category: str | None = None,
    color: str | None = None,
    icon: str | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if title is not None:
        body["title"] = title
    if description is not None:
        body["description"] = description
    if color is not None:
        body["color"] = color
    if icon is not None:
        body["icon"] = icon
    cat_id = _resolve_category_id(category)
    if cat_id is not None:
        body["category_id"] = cat_id
    if not body:
        raise ValueError("provide at least one field to update")
    return _api("PATCH", f"/api/questlines/{questline_id}", body=body)


@server.tool(
    description=(
        "Permanently delete a questline (DELETE /api/questlines/{id}). Quests "
        "that belonged to it are not deleted — detach them first with "
        "update_quest(clear_questline=True) if you want them to survive as "
        "standalone quests, or the API will reject/cascade per its own rules."
    )
)
def delete_questline(questline_id: int) -> dict[str, Any]:
    _api("DELETE", f"/api/questlines/{questline_id}")
    return {"deleted": questline_id}


@server.tool(
    description=(
        "Add one step to an existing quest (POST /api/quests/{id}/steps). "
        "`description` is markdown in the journal (lists, links, code, emphasis). "
        "Returns quest id, progress_label, and steps brief. quiet=true skips overlay toasts."
    )
)
def add_step(
    quest_id: int,
    title: str,
    description: str | None = None,
    progress_total: int = 1,
    progress_current: int = 0,
    sort_order: int | None = None,
    check_command: str | None = None,
    check_interval_seconds: int | None = None,
    wait_previous: bool | None = None,
    run_mode: str | None = None,
    run_on: str | None = None,
    quiet: bool = True,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "title": title,
        "progress_total": max(1, int(progress_total)),
        "progress_current": max(0, int(progress_current)),
    }
    if description is not None:
        body["description"] = description
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    if check_command is not None:
        body["check_command"] = check_command
    if check_interval_seconds is not None:
        body["check_interval_seconds"] = int(check_interval_seconds)
    if wait_previous is not None:
        body["wait_previous"] = bool(wait_previous)
    if run_mode is not None:
        body["run_mode"] = str(run_mode)
    if run_on is not None:
        body["run_on"] = str(run_on)
    q = _api(
        "POST",
        f"/api/quests/{quest_id}/steps",
        query=_tool_query(quiet=quiet),
        body=body,
    )
    return _quest_mutation_result(q)


@server.tool(
    description=(
        "Update quest fields (PATCH /api/quests/{id}). Only pass fields to change. "
        "Use for lifecycle: status=active|expired|frozen|completed|failed|archived "
        "(e.g. archive when blocked / needs clarification). Also title, description "
        "(markdown in the journal), "
        "pinned, overlay_hidden (never shown in the HUD; exclusive with pinned), "
        "significance, sort_order, deadline_at, duration_seconds, "
        "category_id, questline_id (null to detach), automated, tag_ids (replace all, max 5). "
        "quiet=true skips overlay toasts."
    )
)
def update_quest(
    quest_id: int,
    title: str | None = None,
    description: str | None = None,
    status: str | None = None,
    significance: str | None = None,
    pinned: bool | None = None,
    overlay_hidden: bool | None = None,
    sort_order: int | None = None,
    deadline_at: str | None = None,
    duration_seconds: int | None = None,
    category_id: int | None = None,
    questline_id: int | None = None,
    clear_questline: bool = False,
    automated: bool | None = None,
    tag_ids: list[int] | None = None,
    quiet: bool = True,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if title is not None:
        body["title"] = title
    if description is not None:
        body["description"] = description
    if status is not None:
        st = str(status).strip().lower()
        if st not in ALLOWED_STATUS:
            raise ValueError(
                f"bad status {status!r}; expected one of {sorted(ALLOWED_STATUS)}"
            )
        body["status"] = st
    if significance is not None:
        body["significance"] = _check_significance(significance)
    if pinned is not None:
        body["pinned"] = bool(pinned)
    if overlay_hidden is not None:
        body["overlay_hidden"] = bool(overlay_hidden)
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    if deadline_at is not None:
        body["deadline_at"] = deadline_at
    if duration_seconds is not None:
        body["duration_seconds"] = int(duration_seconds)
    if category_id is not None:
        body["category_id"] = int(category_id)
    if clear_questline:
        body["questline_id"] = None
    elif questline_id is not None:
        body["questline_id"] = int(questline_id)
    if automated is not None:
        body["automated"] = bool(automated)
    if tag_ids is not None:
        body["tag_ids"] = [int(x) for x in tag_ids]
    if not body:
        raise ValueError("provide at least one field to update")
    q = _api(
        "PATCH",
        f"/api/quests/{quest_id}",
        query=_tool_query(quiet=quiet),
        body=body,
    )
    return _quest_mutation_result(q)

@server.tool(
    description=(
        "Update fields on one step (PATCH /api/quests/{id}/steps/{step_id}). "
        "Only pass fields to change. `description` is markdown in the journal. "
        "Mark done with progress_current=progress_total "
        "(or progress_current equal to existing total). quiet=true skips overlay toasts."
    )
)
def update_step(
    quest_id: int,
    step_id: int,
    title: str | None = None,
    description: str | None = None,
    progress_current: int | None = None,
    progress_total: int | None = None,
    sort_order: int | None = None,
    check_command: str | None = None,
    check_interval_seconds: int | None = None,
    wait_previous: bool | None = None,
    run_mode: str | None = None,
    run_on: str | None = None,
    quiet: bool = True,
) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if title is not None:
        body["title"] = title
    if description is not None:
        body["description"] = description
    if progress_current is not None:
        body["progress_current"] = int(progress_current)
    if progress_total is not None:
        body["progress_total"] = max(1, int(progress_total))
    if sort_order is not None:
        body["sort_order"] = int(sort_order)
    if check_command is not None:
        body["check_command"] = check_command
    if check_interval_seconds is not None:
        body["check_interval_seconds"] = int(check_interval_seconds)
    if wait_previous is not None:
        body["wait_previous"] = bool(wait_previous)
    if run_mode is not None:
        body["run_mode"] = str(run_mode)
    if run_on is not None:
        body["run_on"] = str(run_on)
    if not body:
        raise ValueError("provide at least one field to update")
    q = _api(
        "PATCH",
        f"/api/quests/{quest_id}/steps/{step_id}",
        query=_tool_query(quiet=quiet),
        body=body,
    )
    return _quest_mutation_result(q)


@server.tool(
    description=(
        "Delete one step (DELETE /api/quests/{id}/steps/{step_id}). "
        "Fails if it is the last step. quiet=true skips overlay toasts."
    )
)
def delete_step(
    quest_id: int,
    step_id: int,
    quiet: bool = True,
) -> dict[str, Any]:
    q = _api(
        "DELETE",
        f"/api/quests/{quest_id}/steps/{step_id}",
        query=_tool_query(quiet=quiet),
    )
    return _quest_mutation_result(q)


_GET_ATTACHMENT_MAX = 512 * 1024
_TEXT_TYPES = (
    "text/",
    "application/json",
    "application/xml",
    "application/javascript",
    "application/x-javascript",
    "application/yaml",
    "application/x-yaml",
    "application/toml",
    "+json",
    "+xml",
)


def _looks_text(content_type: str, raw: bytes) -> bool:
    ct = (content_type or "").lower()
    if any(tok in ct for tok in _TEXT_TYPES):
        return True
    sample = raw[:512]
    if b"\x00" in sample:
        return False
    try:
        sample.decode("utf-8")
        return True
    except UnicodeDecodeError:
        return False


@server.tool(
    description=(
        "Fetch the contents of one attachment. Metadata is already on "
        "list_quests / the get_*_context tools — only call this when you decided the file "
        "is relevant. Pass attachment_id plus exactly one of quest / questline / note. "
        "Optional revision=N downloads that historical version (default: current). "
        "Text is returned as utf-8; anything else as base64. Files over 512 KiB "
        "come back truncated (metadata only plus a note)."
    )
)
def get_attachment(
    attachment_id: int,
    quest: int | None = None,
    questline: int | None = None,
    note: int | None = None,
    revision: int | None = None,
) -> dict[str, Any]:
    chosen = [
        (k, v)
        for k, v in (("quest", quest), ("questline", questline), ("note", note))
        if v is not None
    ]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: quest, questline, note")
    kind, owner_id = chosen[0]
    seg = {"quest": "quests", "questline": "questlines", "note": "notes"}[kind]
    meta_rows = _api_get(f"/api/{seg}/{owner_id}/attachments", {"stat": "0"}) or []
    meta = next((a for a in meta_rows if a.get("id") == attachment_id), None)
    path = f"/api/{seg}/{owner_id}/attachments/{attachment_id}"
    if revision is not None:
        path = f"{path}?revision={int(revision)}"
    raw, _hdrs = _api_raw("GET", path)
    out: dict[str, Any] = {
        "id": attachment_id,
        "owner_type": kind,
        "owner_id": owner_id,
        "filename": (meta or {}).get("filename"),
        "size_bytes": (meta or {}).get("size_bytes", len(raw)),
        "content_type": (meta or {}).get("content_type_detected")
        or (meta or {}).get("content_type_declared"),
        "comment": (meta or {}).get("comment"),
        "hidden": bool((meta or {}).get("hidden")),
        "revision": revision if revision is not None else (meta or {}).get("revision"),
        "revision_count": (meta or {}).get("revision_count"),
        "truncated": False,
    }
    if len(raw) > _GET_ATTACHMENT_MAX:
        out["truncated"] = True
        out["note"] = (
            f"file is {len(raw)} bytes; contents omitted above "
            f"{_GET_ATTACHMENT_MAX} bytes"
        )
        return out
    if _looks_text(str(out.get("content_type") or ""), raw):
        out["text"] = raw.decode("utf-8", errors="replace")
        out["encoding"] = "utf-8"
    else:
        out["base64"] = base64.standard_b64encode(raw).decode("ascii")
        out["encoding"] = "base64"
    return out


@server.tool(
    description=(
        "Hide or unhide an attachment (PATCH …/attachments/{id} hidden=bool). "
        "A hidden attachment is not deleted: it disappears from quest/note "
        "blocks, markdown images, @-suggestions, PDF export and the *_context "
        "tools, and stays visible only on the journal's Attachments tab "
        "(crossed-eye icon), where it can be restored. Prefer this over "
        "deleting when a file is merely stale. get_attachment still reads a "
        "hidden file by id. Pass attachment_id, hidden (true/false) and "
        "exactly one of quest / questline / note."
    )
)
def set_attachment_hidden(
    attachment_id: int,
    hidden: bool,
    quest: int | None = None,
    questline: int | None = None,
    note: int | None = None,
) -> dict[str, Any]:
    chosen = [
        (k, v)
        for k, v in (("quest", quest), ("questline", questline), ("note", note))
        if v is not None
    ]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: quest, questline, note")
    kind, owner_id = chosen[0]
    seg = {"quest": "quests", "questline": "questlines", "note": "notes"}[kind]
    row = _api(
        "PATCH",
        f"/api/{seg}/{owner_id}/attachments/{int(attachment_id)}",
        body={"hidden": bool(hidden)},
    )
    return {
        "id": row.get("id"),
        "filename": row.get("filename"),
        "hidden": row.get("hidden"),
    }


@server.tool(
    description=(
        "Upload a custom icon image (POST /api/{questlines|notes}/{id}/icon) "
        "for a questline or note, replacing its string icon code with an "
        "uploaded picture. Pass exactly one of questline / note plus a local "
        "file_path (png/jpg/webp/gif/svg, max 512 KiB; the type is taken from "
        "the file extension). This reads the file from the local "
        "filesystem where the MCP server runs, not from the conversation."
    )
)
def set_icon(
    file_path: str,
    questline: int | None = None,
    note: int | None = None,
) -> dict[str, Any]:
    chosen = [(k, v) for k, v in (("questline", questline), ("note", note)) if v is not None]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: questline, note")
    kind, owner_id = chosen[0]
    seg = {"questline": "questlines", "note": "notes"}[kind]
    return _api_upload_file(f"/api/{seg}/{owner_id}/icon", file_path)


@server.tool(
    description=(
        "Upload a local file as an attachment (POST /api/{quests|questlines|"
        "notes}/{id}/attachments) on a quest, questline, or note. Pass a "
        "local file_path (this reads from the filesystem where the MCP "
        "server runs, not from the conversation) plus exactly one of quest / "
        "questline / note, and an optional comment. Optional as_version_of="
        "attachment_id uploads a *new revision* of that card "
        "(POST …/attachments/{id}/revisions) instead of a sibling file — use "
        "this instead of dumping *_v2.c next to the old one. Requires the "
        "API's WebDAV storage and ClamAV scanning; rejected if the scan fails. "
        "Use get_attachment to read a file back afterward."
    )
)
def upload_attachment(
    file_path: str,
    quest: int | None = None,
    questline: int | None = None,
    note: int | None = None,
    comment: str | None = None,
    as_version_of: int | None = None,
) -> dict[str, Any]:
    chosen = [
        (k, v)
        for k, v in (("quest", quest), ("questline", questline), ("note", note))
        if v is not None
    ]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: quest, questline, note")
    kind, owner_id = chosen[0]
    seg = {"quest": "quests", "questline": "questlines", "note": "notes"}[kind]
    extra = {"comment": comment} if comment else None
    if as_version_of is not None:
        return _api_upload_file(
            f"/api/{seg}/{owner_id}/attachments/{int(as_version_of)}/revisions",
            file_path,
            extra_fields=extra,
        )
    return _api_upload_file(
        f"/api/{seg}/{owner_id}/attachments",
        file_path,
        extra_fields=extra,
    )


@server.tool(
    description=(
        "Remove a questline's or note's uploaded custom icon (DELETE "
        "/api/{questlines|notes}/{id}/icon), reverting to its plain string "
        "icon code. Pass exactly one of questline / note."
    )
)
def delete_icon(
    questline: int | None = None,
    note: int | None = None,
) -> dict[str, Any]:
    chosen = [(k, v) for k, v in (("questline", questline), ("note", note)) if v is not None]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: questline, note")
    kind, owner_id = chosen[0]
    seg = {"questline": "questlines", "note": "notes"}[kind]
    _api("DELETE", f"/api/{seg}/{owner_id}/icon")
    return {"cleared_icon": {kind: owner_id}}


@server.tool(
    description=(
        "Set the accent/line color of a questline or note (PATCH `color` on "
        "/api/{questlines|notes}/{id}) — the swatch shown behind its icon in "
        "the tree/HUD. Pass a hex string (e.g. '#c47a20'; a leading '#' is "
        "added if missing) plus exactly one of questline / note. This is the "
        "string `color` field, separate from an uploaded custom icon image "
        "(see set_icon). For a questline it's the same field "
        "update_questline(color=...) sets; set_icon_color also covers notes, "
        "whose update_note takes no color."
    )
)
def set_icon_color(
    color: str,
    questline: int | None = None,
    note: int | None = None,
) -> dict[str, Any]:
    chosen = [(k, v) for k, v in (("questline", questline), ("note", note)) if v is not None]
    if len(chosen) != 1:
        raise ValueError("provide exactly one of: questline, note")
    hexval = (color or "").strip()
    if not hexval:
        raise ValueError("color must be a non-empty hex string like '#c47a20'")
    if not hexval.startswith("#"):
        hexval = f"#{hexval}"
    kind, owner_id = chosen[0]
    seg = {"questline": "questlines", "note": "notes"}[kind]
    return _api("PATCH", f"/api/{seg}/{owner_id}", body={"color": hexval})


def main(argv: list[str] | None = None) -> None:
    global API_BASE
    load_dotenv_files()
    parser = argparse.ArgumentParser(prog="quests-mcp", description="Quests MCP server")
    parser.add_argument(
        "--api",
        default=None,
        metavar="URL",
        help=f"Quests API base (else QUESTS_API or {API_BASE})",
    )
    args = parser.parse_args(list(sys.argv[1:] if argv is None else argv))

    if args.api:
        API_BASE = str(args.api).rstrip("/")
    elif os.environ.get("QUESTS_API"):
        API_BASE = os.environ["QUESTS_API"].rstrip("/")

    server.run(transport="stdio")


if __name__ == "__main__":
    main()
