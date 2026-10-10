"""Quests CLI launcher → Go binary ``go/bin/quests``.

Native Python remains only for ``llm-add`` (``QUESTS_CLI_NATIVE=1``).
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

from quests.config import HOST, PORT

API_BASE = (os.environ.get("QUESTS_API") or f"http://{HOST}:{PORT}").rstrip("/")


class CliError(Exception):
    def __init__(self, message: str, code: int = 1) -> None:
        super().__init__(message)
        self.code = code


def emit(data: Any, *, as_json: bool, text: str | None = None) -> None:
    if as_json:
        print(json.dumps(data, ensure_ascii=False, indent=2, default=str))
    elif text is not None:
        print(text)
    else:
        print(data)


def emit_error(message: str, *, as_json: bool, code: int = 1) -> int:
    if as_json:
        print(
            json.dumps({"ok": False, "error": message}, ensure_ascii=False),
            file=sys.stderr,
        )
    else:
        print(f"error: {message}", file=sys.stderr)
    return code


def api_request(
    method: str,
    path: str,
    *,
    body: dict[str, Any] | None = None,
    query: dict[str, Any] | None = None,
) -> Any:
    url = f"{API_BASE}{path}"
    if query:
        q = {k: v for k, v in query.items() if v is not None}
        if q:
            url = f"{url}?{urllib.parse.urlencode(q)}"
    data = None
    headers = {"Accept": "application/json", "X-Quests-Source": "cli"}
    # Bearer token for an instance with accounts enabled; empty when open.
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
            if not raw:
                return None
            return json.loads(raw.decode("utf-8"))
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(detail)
            detail = parsed.get("detail", detail)
        except json.JSONDecodeError:
            pass
        raise CliError(f"API {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise CliError(
            f"не удалось связаться с API ({API_BASE}): {e.reason}. "
            "Запусти сервер: ./scripts/run-server.sh"
        ) from e


def cmd_llm_add(ns: argparse.Namespace) -> int:
    """Free-form text → LLM draft → POST /api/quests."""
    from quests.llm import (
        LlmError,
        draft_to_create_body,
        extract_quest_draft_sync,
        format_draft_preview,
    )
    from quests.llm.config import load_llm_settings

    text = " ".join(ns.text).strip() if isinstance(ns.text, list) else str(ns.text).strip()
    if not text:
        raise CliError("нужен текст описания")

    settings = load_llm_settings()
    if not bool(getattr(ns, "json", False)):
        if settings.provider == "groq":
            print(f"Groq ({settings.model})…", file=sys.stderr)
        else:
            print(
                f"Ollama ({settings.model} @ {settings.base_url})…",
                file=sys.stderr,
            )
    try:
        bundle = extract_quest_draft_sync(text, settings=settings)
    except LlmError as e:
        raise CliError(str(e)) from e

    as_json = bool(getattr(ns, "json", False))
    if bundle.needs_clarification and (bundle.clarify_question or "").strip():
        draft = bundle.primary
        if as_json:
            emit(
                {
                    "ok": False,
                    "needs_clarification": True,
                    "question": bundle.clarify_question,
                    "draft": draft.model_dump(),
                    "variations": [d.model_dump() for d in bundle.variations],
                },
                as_json=True,
            )
            return 2
        print(format_draft_preview(draft, index=0, total=len(bundle.variations)))
        print(f"\nуточнение: {bundle.clarify_question}", file=sys.stderr)
        return 2

    pick = max(0, int(getattr(ns, "variant", 0) or 0))
    pick = min(pick, len(bundle.variations) - 1)
    draft = bundle.variations[pick]
    cats = api_request("GET", "/api/categories") or []
    body = draft_to_create_body(draft, categories=cats)

    if not bool(getattr(ns, "yes", False)) and not as_json:
        for i, d in enumerate(bundle.variations):
            mark = "→ " if i == pick else "  "
            print(mark + format_draft_preview(d, index=i, total=len(bundle.variations)))
            print()
        try:
            ans = input(f"создать вариант {pick + 1}? [y/N] ").strip().lower()
        except EOFError:
            ans = ""
        if ans not in {"y", "yes", "д", "да"}:
            print("отменено")
            return 1

    q = api_request("POST", "/api/quests", body=body)
    if as_json:
        emit(
            {
                "ok": True,
                "draft": draft.model_dump(),
                "variations": [d.model_dump() for d in bundle.variations],
                "picked": pick,
                "quest": q,
            },
            as_json=True,
        )
    else:
        print(f"создан #{q['id']}: {q['title']}")
    return 0


def _llm_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="quests llm-add")
    p.add_argument("--json", action="store_true")
    p.add_argument("--api", default=None)
    p.add_argument("text", nargs="+")
    p.add_argument("-y", "--yes", action="store_true")
    p.add_argument("--variant", type=int, default=0)
    return p


def main(argv: list[str] | None = None) -> int:
    from quests.envload import load_dotenv_files

    load_dotenv_files()
    argv_list = list(sys.argv[1:] if argv is None else argv)

    if os.environ.get("QUESTS_CLI_NATIVE") != "1":
        return _exec_go_cli(argv_list)

    return _main_llm_add(argv_list)


def _exec_go_cli(argv_list: list[str]) -> int:
    from pathlib import Path

    root = Path(__file__).resolve().parents[2]
    binary = Path(os.environ.get("QUESTS_CLI_BIN") or root / "go" / "bin" / "quests")
    env = os.environ.copy()
    env.setdefault("QUESTS_ROOT", str(root))
    if not binary.is_file():
        build = root / "scripts" / "build-cli.sh"
        if build.is_file():
            import subprocess

            subprocess.check_call([str(build)], env=env)
        if not binary.is_file():
            print(
                f"error: Go CLI binary not found at {binary}; run `just build-cli`",
                file=sys.stderr,
            )
            return 1
    os.execvpe(str(binary), [str(binary), *argv_list], env)
    return 1


def _main_llm_add(argv_list: list[str]) -> int:
    # Strip leading subcommand when Go shells: python -m quests.cli llm-add …
    kind = argv_list[0] if argv_list else ""
    if kind in {"llm-add", "add-llm", "new-llm"}:
        argv_list = argv_list[1:]

    parser = _llm_parser()
    runner = cmd_llm_add

    try:
        args = parser.parse_args(argv_list)
    except SystemExit as e:
        return int(e.code or 0)

    global API_BASE
    if args.api:
        API_BASE = str(args.api).rstrip("/")

    try:
        return int(runner(args) or 0)
    except CliError as e:
        return emit_error(str(e), as_json=bool(args.json), code=e.code)
    except BrokenPipeError:
        return 0


if __name__ == "__main__":
    raise SystemExit(main())
