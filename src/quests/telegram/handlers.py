"""Bot command handlers: list, create dialog, status/step callbacks."""

from __future__ import annotations

import logging
import re
from datetime import UTC, datetime, timedelta
from typing import Any

from aiogram import F, Router
from aiogram.exceptions import TelegramBadRequest
from aiogram.filters import Command, CommandStart, StateFilter
from aiogram.fsm.context import FSMContext
from aiogram.fsm.state import State, StatesGroup
from aiogram.types import CallbackQuery, Message

from quests.telegram.actions_preview import (
    format_actions_preview,
    history_to_prompt_text,
)
from quests.telegram.api_client import ApiError, QuestsApi
from quests.telegram.formatters import format_active_by_category, format_quest_card, status_label
from quests.telegram.keyboards import (
    BTN_CANCEL,
    BTN_HELP,
    BTN_LIST,
    BTN_NEW,
    BTN_NEW_LLM,
    _is_closed,
    category_pick_keyboard,
    llm_confirm_keyboard,
    main_reply_keyboard,
    quest_keyboard,
    quest_pick_keyboard,
)
from quests.telegram.resilience import tg_retry, tg_soft
from quests.telegram.settings import TgSettings
from quests.telegram.store import ChatRegistry
from quests.timeutil import to_utc_iso

log = logging.getLogger("quests.telegram.handlers")

_NAV_BUTTONS = {BTN_LIST, BTN_NEW, BTN_NEW_LLM, BTN_HELP, BTN_CANCEL}

HELP = (
    "<b>Quests · Telegram</b>\n"
    f"{BTN_LIST} / /list — активные: раздел → квестлайн → квесты\n"
    f"{BTN_NEW} / /new — создать задачу\n"
    f"{BTN_NEW_LLM} / /new-llm — текст или голос → план действий (как «Команда» в вебе)\n"
    "Или просто напиши задачу в чат (больше 10 символов).\n"
    "На карточке квеста кнопки quest=/questline=/step= копируют ссылку в буфер.\n"
    "/quest &lt;id&gt; — карточка, статус и шаги\n"
    f"{BTN_CANCEL} / /cancel — отменить диалог\n"
    f"{BTN_HELP} / /help — справка"
)


class CreateQuest(StatesGroup):
    title = State()
    category = State()
    deadline = State()


class CreateLlm(StatesGroup):
    text = State()
    clarify = State()
    confirm = State()


def _parse_relative_duration(raw: str) -> int | None:
    """Parse 30m / 2h / 1d → seconds. None if not matched."""
    m = re.fullmatch(r"\s*(\d+)\s*([mhd])\s*", raw.strip(), flags=re.IGNORECASE)
    if not m:
        return None
    n = int(m.group(1))
    unit = m.group(2).lower()
    if unit == "m":
        return max(60, n * 60)
    if unit == "h":
        return max(60, n * 3600)
    return max(60, n * 86400)


def _step_is_done(step: dict) -> bool:
    if step.get("done") is True:
        return True
    try:
        return int(step.get("progress_current") or 0) >= int(
            step.get("progress_total") or 1
        )
    except (TypeError, ValueError):
        return False


async def _edit_quest_card(
    query: CallbackQuery, q: dict, *, page: int = 0, expanded: bool = False
) -> None:
    text = format_quest_card(q)
    markup = quest_keyboard(q, page=page, expanded=expanded)
    msg = query.message
    if msg is None:
        return
    try:
        await tg_retry(
            lambda: msg.edit_text(text, reply_markup=markup),  # type: ignore[union-attr]
            label="edit-card",
        )
        return
    except TelegramBadRequest as e:
        # Same content → Telegram rejects edit; do not spam a duplicate card.
        err = str(e).lower()
        if "message is not modified" in err:
            return
    except Exception:
        log.warning("edit-card failed; will replace message", exc_info=True)

    chat = msg.chat
    chat_id = chat.id if chat else None
    old_id = msg.message_id
    if chat_id is not None and old_id is not None:
        await tg_soft(
            lambda: query.bot.delete_message(int(chat_id), int(old_id)),  # type: ignore[union-attr]
            label="replace-card-del",
        )
    if chat_id is None:
        return
    await tg_retry(
        lambda: query.bot.send_message(  # type: ignore[union-attr]
            int(chat_id),
            text,
            reply_markup=markup,
        ),
        label="replace-card-send",
    )


# FSM key: bot message_ids to wipe when create/LLM dialog ends.
_DIALOG_BOT_MSGS = "dialog_bot_msgs"


async def _track_bot_msg(state: FSMContext, msg: Message | None) -> None:
    if msg is None or msg.message_id is None:
        return
    data = await state.get_data()
    ids = [int(x) for x in (data.get(_DIALOG_BOT_MSGS) or [])]
    mid = int(msg.message_id)
    if mid not in ids:
        ids.append(mid)
    await state.update_data(**{_DIALOG_BOT_MSGS: ids})


async def _purge_dialog(
    bot: Any,
    state: FSMContext,
    chat_id: int | None,
) -> None:
    """Delete tracked bot messages from the create/LLM dialog, then clear FSM.

    User messages are left alone: in private chats Telegram forbids bots
    deleting anyone else's messages.
    """
    data = await state.get_data()
    ids = [int(x) for x in (data.get(_DIALOG_BOT_MSGS) or [])]
    await state.clear()
    if bot is None or chat_id is None:
        return
    for mid in ids:
        await tg_soft(
            lambda m=mid: bot.delete_message(int(chat_id), m),
            label=f"purge:{chat_id}:{mid}",
        )


async def _on_quest_callback_error(query: CallbackQuery, e: ApiError) -> None:
    """Alert; if quest gone (404), drop the stale inline message."""
    await query.answer(str(e), show_alert=True)
    if e.status != 404:
        return
    msg = query.message
    if msg is None or query.bot is None:
        return
    chat = msg.chat
    if chat is None or msg.message_id is None:
        return
    await tg_soft(
        lambda: query.bot.delete_message(int(chat.id), int(msg.message_id)),
        label="missing-quest-del",
    )


def build_router(
    *,
    api: QuestsApi,
    settings: TgSettings,
    chats: ChatRegistry,
) -> Router:
    router = Router(name="quests")
    reply_kb = main_reply_keyboard()

    def allowed(user_id: int | None) -> bool:
        return user_id is not None and int(user_id) in settings.user_ids

    async def _say(
        message: Message,
        state: FSMContext,
        text: str,
        **kwargs: Any,
    ) -> Message:
        msg = await tg_retry(
            lambda: message.answer(text, **kwargs),
            label="answer",
        )
        await _track_bot_msg(state, msg)
        return msg

    async def _say_chat(
        bot: Any,
        state: FSMContext,
        chat_id: int,
        text: str,
        **kwargs: Any,
    ) -> Message:
        msg = await tg_retry(
            lambda: bot.send_message(chat_id, text, **kwargs),
            label="send",
        )
        await _track_bot_msg(state, msg)
        return msg

    @router.message.middleware()
    async def auth_message(handler, event: Message, data: dict[str, Any]):
        uid = event.from_user.id if event.from_user else None
        if not allowed(uid):
            log.info("ignored message from user_id=%s", uid)
            return None
        if uid is not None and event.chat:
            chats.remember(int(uid), int(event.chat.id))
        return await handler(event, data)

    @router.callback_query.middleware()
    async def auth_callback(handler, event: CallbackQuery, data: dict[str, Any]):
        uid = event.from_user.id if event.from_user else None
        if not allowed(uid):
            await event.answer("нет доступа", show_alert=True)
            return None
        if uid is not None and event.message and event.message.chat:
            chats.remember(int(uid), int(event.message.chat.id))
        return await handler(event, data)

    async def do_list(message: Message) -> None:
        try:
            quests = await api.list_quests(status="active")
        except ApiError as e:
            await message.answer(f"Ошибка API: {e}", reply_markup=reply_kb)
            return
        text = format_active_by_category(quests)
        await message.answer(
            text,
            reply_markup=quest_pick_keyboard(quests) if quests else reply_kb,
        )

    async def do_help(message: Message) -> None:
        await message.answer(HELP, reply_markup=reply_kb)

    @router.message(CommandStart())
    async def cmd_start(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await message.answer(HELP, reply_markup=reply_kb)

    @router.message(Command("help"))
    @router.message(F.text == BTN_HELP)
    async def cmd_help(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await do_help(message)

    @router.message(Command("cancel"))
    @router.message(F.text == BTN_CANCEL)
    async def cmd_cancel(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await message.answer("Отменено.", reply_markup=reply_kb)

    @router.message(Command("list"))
    @router.message(F.text == BTN_LIST)
    async def cmd_list(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await do_list(message)

    @router.message(Command("quest"))
    async def cmd_quest(message: Message) -> None:
        parts = (message.text or "").split(maxsplit=1)
        if len(parts) < 2 or not parts[1].strip().isdigit():
            await message.answer(
                "Использование: /quest &lt;id&gt;", reply_markup=reply_kb
            )
            return
        qid = int(parts[1].strip())
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await message.answer(f"Ошибка API: {e}", reply_markup=reply_kb)
            return
        await message.answer(
            format_quest_card(q),
            reply_markup=quest_keyboard(q),
        )

    @router.callback_query(F.data.startswith("qo:"))
    async def cb_open(query: CallbackQuery) -> None:
        raw = (query.data or "").split(":", 1)[-1]
        if not raw.isdigit():
            await query.answer("Неверный номер")
            return
        qid = int(raw)
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        await query.message.answer(  # type: ignore[union-attr]
            format_quest_card(q),
            reply_markup=quest_keyboard(q),
        )
        await query.answer()

    @router.callback_query(F.data.startswith("qe:"))
    async def cb_edit_expand(query: CallbackQuery) -> None:
        raw = (query.data or "").split(":", 1)[-1]
        if not raw.isdigit():
            await query.answer("Неверный номер")
            return
        qid = int(raw)
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        await _edit_quest_card(query, q, page=0, expanded=True)
        await query.answer()

    @router.callback_query(F.data.startswith("qr:"))
    async def cb_refresh(query: CallbackQuery) -> None:
        parts = (query.data or "").split(":")
        if len(parts) < 2 or not parts[1].isdigit():
            await query.answer("Неверный номер")
            return
        qid = int(parts[1])
        page = int(parts[2]) if len(parts) > 2 and parts[2].isdigit() else 0
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        # Не форсить expanded: если снаружи уже completed — свернуть в «Редактировать».
        await _edit_quest_card(query, q, page=page, expanded=not _is_closed(q))
        await query.answer("обновлено")

    @router.callback_query(F.data.startswith("qp:"))
    async def cb_page(query: CallbackQuery) -> None:
        parts = (query.data or "").split(":")
        if len(parts) != 3 or not parts[1].isdigit() or not parts[2].isdigit():
            await query.answer("Неверная страница")
            return
        qid = int(parts[1])
        page = int(parts[2])
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        await _edit_quest_card(query, q, page=page, expanded=True)
        await query.answer()

    @router.callback_query(F.data.startswith("qs:"))
    async def cb_status(query: CallbackQuery) -> None:
        parts = (query.data or "").split(":")
        if len(parts) != 3 or not parts[1].isdigit():
            await query.answer("Не удалось распознать действие")
            return
        qid = int(parts[1])
        new_status = parts[2]
        if new_status not in {"active", "completed", "failed", "expired", "frozen"}:
            await query.answer("Неизвестный статус")
            return
        try:
            current = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        if new_status == "completed" and str(current.get("status")) == "completed":
            await _edit_quest_card(query, current, page=0, expanded=False)
            await query.answer()
            return
        try:
            q = await api.patch_quest(qid, {"status": new_status})
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        await _edit_quest_card(
            query, q, page=0, expanded=not _is_closed(q)
        )
        await query.answer(f"→ {status_label(new_status)}")

    @router.callback_query(F.data.startswith("qt:"))
    async def cb_toggle_step(query: CallbackQuery) -> None:
        # qt:<quest_id>:<step_id>:<page>
        parts = (query.data or "").split(":")
        if (
            len(parts) != 4
            or not parts[1].isdigit()
            or not parts[2].isdigit()
            or not parts[3].isdigit()
        ):
            await query.answer("Неверный шаг")
            return
        qid = int(parts[1])
        sid = int(parts[2])
        page = int(parts[3])
        try:
            q = await api.get_quest(qid)
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        step = next((s for s in (q.get("steps") or []) if int(s["id"]) == sid), None)
        if step is None:
            await query.answer("шаг не найден", show_alert=True)
            return
        total = max(1, int(step.get("progress_total") or 1))
        new_cur = 0 if _step_is_done(step) else total
        try:
            q = await api.patch_step(qid, sid, {"progress_current": new_cur})
        except ApiError as e:
            await _on_quest_callback_error(query, e)
            return
        await _edit_quest_card(query, q, page=page, expanded=True)
        await query.answer("✓" if new_cur >= total else "сброс")

    # ── create dialog ─────────────────────────────────────────────────────

    @router.message(Command("new"))
    @router.message(F.text == BTN_NEW)
    async def cmd_new(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await state.set_state(CreateQuest.title)
        await _say(
            message,
            state,
            "Новая задача. Пришли <b>название</b> (или Отмена).",
            reply_markup=reply_kb,
        )

    @router.message(StateFilter(CreateQuest.title), F.text)
    async def create_title(message: Message, state: FSMContext) -> None:
        title = (message.text or "").strip()
        if title in _NAV_BUTTONS:
            return
        if not title or title.startswith("/"):
            await _say(message, state, "Нужен обычный текст названия.")
            return
        if len(title) > 200:
            await _say(message, state, "Слишком длинно (макс. 200).")
            return
        await state.update_data(title=title)
        try:
            cats = await api.list_categories()
        except ApiError as e:
            chat_id = message.chat.id if message.chat else None
            await _purge_dialog(message.bot, state, chat_id)
            await message.answer(f"Ошибка API: {e}", reply_markup=reply_kb)
            return
        await state.set_state(CreateQuest.category)
        await _say(
            message,
            state,
            f"Раздел для «{title}»:",
            reply_markup=category_pick_keyboard(cats),
        )

    @router.callback_query(StateFilter(CreateQuest.category), F.data.startswith("cc:"))
    async def create_category(query: CallbackQuery, state: FSMContext) -> None:
        token = (query.data or "").split(":", 1)[-1]
        chat = query.message.chat if query.message else None  # type: ignore[union-attr]
        chat_id = chat.id if chat else None
        if token == "cancel":
            await _purge_dialog(query.bot, state, chat_id)
            if chat_id is not None:
                await tg_retry(
                    lambda: query.bot.send_message(  # type: ignore[union-attr]
                        chat_id, "Отменено.", reply_markup=reply_kb
                    ),
                    label="create-cancel",
                )
            await query.answer()
            return
        if token == "0":
            await state.update_data(category_id=None)
        elif token.isdigit():
            await state.update_data(category_id=int(token))
        else:
            await query.answer("Неверная категория")
            return
        # Category picker message is part of the dialog — keep tracked.
        if query.message is not None:
            await _track_bot_msg(state, query.message)  # type: ignore[arg-type]
        await state.set_state(CreateQuest.deadline)
        if chat_id is not None:
            await _say_chat(
                query.bot,
                state,
                chat_id,
                "Срок? Примеры: <code>1h</code>, <code>30m</code>, <code>2d</code> "
                "или <code>-</code> без срока.",
                reply_markup=reply_kb,
            )
        await query.answer()

    @router.message(StateFilter(CreateQuest.deadline), F.text)
    async def create_deadline(message: Message, state: FSMContext) -> None:
        raw = (message.text or "").strip()
        if raw in _NAV_BUTTONS:
            return
        data = await state.get_data()
        body: dict[str, Any] = {
            "title": data["title"],
            "status": "active",
            "significance": "common",
        }
        if data.get("category_id") is not None:
            body["category_id"] = int(data["category_id"])

        if raw not in {"-", "—", "нет", "no", ""}:
            seconds = _parse_relative_duration(raw)
            if seconds is None:
                await _say(
                    message,
                    state,
                    "Не понял срок. Формат: 30m / 2h / 1d или -",
                )
                return
            now = datetime.now(UTC)
            deadline = now + timedelta(seconds=seconds)
            body["deadline_at"] = to_utc_iso(deadline)
            body["duration_seconds"] = seconds

        chat_id = message.chat.id if message.chat else None
        try:
            q = await api.create_quest(body)
        except ApiError as e:
            await _purge_dialog(message.bot, state, chat_id)
            await message.answer(f"Не удалось создать: {e}", reply_markup=reply_kb)
            return

        await _purge_dialog(message.bot, state, chat_id)
        title = _esc_html(str(q.get("title") or data.get("title") or ""))
        await tg_retry(
            lambda: message.answer(f"Создано задание «{title}»", reply_markup=reply_kb),
            label="created",
        )

    # ── LLM free-form → Go action-batch (same as web «Команда») ───────────

    async def _run_llm(
        message: Message,
        state: FSMContext,
        user_text: str,
        *,
        history: list[tuple[str, str]] | None = None,
    ) -> None:
        wait = await tg_retry(
            lambda: message.answer(
                "Думаю (API · action-batch)…",
                reply_markup=reply_kb,
            ),
            label="llm-wait",
        )
        chat_id = message.chat.id if message.chat else None
        prompt = history_to_prompt_text(user_text, history)
        try:
            # one clarifying question per request: after an answer the model must commit
            res = await api.preview_actions(
                prompt, no_clarify=any(r == "assistant" for r, _ in (history or []))
            )
        except ApiError as e:
            reason = str(e)
            await tg_soft(lambda: wait.delete(), label="llm-wait-del")
            await _purge_dialog(message.bot, state, chat_id)
            await tg_retry(
                lambda: message.answer(f"LLM: {reason}", reply_markup=reply_kb),
                label="llm-err",
            )
            return
        await tg_soft(lambda: wait.delete(), label="llm-wait-del")

        if res.get("needs_clarification"):
            q = (res.get("clarify_question") or "Уточни, пожалуйста.").strip()
            hist = list(history or [])
            hist.append(("user", user_text))
            hist.append(("assistant", q))
            await state.update_data(llm_history=hist)
            await state.set_state(CreateLlm.clarify)
            await _say(
                message,
                state,
                f"Уточни: {_esc_html(q)}",
                reply_markup=reply_kb,
            )
            return

        batch = res.get("batch") or {}
        preview = list(res.get("preview") or [])
        if not batch.get("actions"):
            await _purge_dialog(message.bot, state, chat_id)
            await tg_retry(
                lambda: message.answer(
                    "Модель не предложила действий. Переформулируй.",
                    reply_markup=reply_kb,
                ),
                label="llm-empty",
            )
            return

        await state.update_data(llm_batch=batch, llm_preview=preview)
        await state.set_state(CreateLlm.confirm)
        await _say(
            message,
            state,
            format_actions_preview(batch, preview),
            reply_markup=llm_confirm_keyboard(index=0, total=1),
            parse_mode="HTML",
        )

    @router.message(Command("new_llm"))
    @router.message(F.text.regexp(r"(?s)^/new-llm(?:@\w+)?(?:\s|$)"))
    @router.message(F.text == BTN_NEW_LLM)
    async def cmd_new_llm(message: Message, state: FSMContext) -> None:
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        text = message.text or ""
        inline = ""
        if text.startswith("/"):
            parts = text.split(maxsplit=1)
            if len(parts) > 1:
                inline = parts[1].strip()
        if inline:
            await state.set_state(CreateLlm.text)
            await _run_llm(message, state, inline)
            return
        await state.set_state(CreateLlm.text)
        await _say(
            message,
            state,
            "Опиши задачу текстом (или Отмена).\n"
            "Можно создавать/менять квесты и квестлайны, как «Команда» в журнале.\n"
            "Ссылки: скопируй quest=N / questline=N с карточки.\n"
            "Пример: «на час разобрать почту, раздел работа»",
            reply_markup=reply_kb,
        )

    @router.message(StateFilter(CreateLlm.text), F.text)
    async def llm_text(message: Message, state: FSMContext) -> None:
        text = (message.text or "").strip()
        if text in _NAV_BUTTONS:
            return
        if not text or text.startswith("/"):
            await _say(message, state, "Нужен текст.")
            return
        await _run_llm(message, state, text)

    @router.message(StateFilter(CreateLlm.clarify), F.text)
    async def llm_clarify(message: Message, state: FSMContext) -> None:
        text = (message.text or "").strip()
        if text in _NAV_BUTTONS:
            return
        if not text or text.startswith("/"):
            await _say(message, state, "Нужен ответ текстом.")
            return
        data = await state.get_data()
        history = list(data.get("llm_history") or [])
        await _run_llm(message, state, text, history=history)

    @router.callback_query(StateFilter(CreateLlm.confirm), F.data == "llm:no")
    async def llm_cancel(query: CallbackQuery, state: FSMContext) -> None:
        chat = query.message.chat if query.message else None  # type: ignore[union-attr]
        chat_id = chat.id if chat else None
        if query.message is not None:
            await _track_bot_msg(state, query.message)  # type: ignore[arg-type]
        await _purge_dialog(query.bot, state, chat_id)
        if chat_id is not None:
            await tg_retry(
                lambda: query.bot.send_message(  # type: ignore[union-attr]
                    chat_id, "Отменено.", reply_markup=reply_kb
                ),
                label="llm-cancel",
            )
        await query.answer("отменено")

    @router.callback_query(StateFilter(CreateLlm.confirm), F.data == "llm:prev")
    async def llm_prev(query: CallbackQuery, state: FSMContext) -> None:
        await query.answer("один план")

    @router.callback_query(StateFilter(CreateLlm.confirm), F.data == "llm:next")
    async def llm_next(query: CallbackQuery, state: FSMContext) -> None:
        await query.answer("один план")

    @router.callback_query(StateFilter(CreateLlm.confirm), F.data == "llm:ok")
    async def llm_ok(query: CallbackQuery, state: FSMContext) -> None:
        data = await state.get_data()
        batch = data.get("llm_batch")
        chat = query.message.chat if query.message else None  # type: ignore[union-attr]
        chat_id = chat.id if chat else None
        if query.message is not None:
            await _track_bot_msg(state, query.message)  # type: ignore[arg-type]
        if not batch or not batch.get("actions"):
            await _purge_dialog(query.bot, state, chat_id)
            await query.answer("план потерян", show_alert=True)
            return
        try:
            result = await api.apply_actions(batch)
        except ApiError as e:
            await query.answer(str(e), show_alert=True)
            return
        await _purge_dialog(query.bot, state, chat_id)
        n = len((result or {}).get("results") or batch.get("actions") or [])
        if chat_id is not None:
            await tg_retry(
                lambda: query.bot.send_message(  # type: ignore[union-attr]
                    chat_id,
                    f"Применено действий: {n}",
                    reply_markup=reply_kb,
                ),
                label="llm-applied",
            )
        await query.answer("готово")

    @router.message(F.text)
    async def unhandled_text(message: Message, state: FSMContext) -> None:
        """Idle long text → LLM; otherwise help.

        Telegram has no «chat opened» event, so the reply keyboard can stay
        hidden until some message is sent. Treat a sentence as a quest draft.
        """
        text = (message.text or "").strip()
        current = await state.get_state()
        if (
            current is None
            and text
            and not text.startswith("/")
            and text not in _NAV_BUTTONS
            and len(text) > 10
        ):
            await state.set_state(CreateLlm.text)
            await _run_llm(message, state, text)
            return
        await _purge_dialog(message.bot, state, message.chat.id if message.chat else None)
        await do_help(message)

    return router


def _esc_html(text: str) -> str:
    return (
        text.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
    )
