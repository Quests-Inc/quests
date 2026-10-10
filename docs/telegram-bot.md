# Telegram bot and voice input (optional client)

The bot is **not part of the platform**: it is an ordinary API client, like the HUD or the CLI. It talks to Quests with one
API token, so it works in exactly one workspace, for the Telegram ids you whitelist. The hosted multi-user service does not run
one for others; run it yourself, on your own hardware, against your own account.

What it gives you: push messages for quest events and deadline windows, commands and the action assistant from a chat, and
voice notes (through the optional STT sidecar, `faster-whisper`, a ~1.5 GB image).

## Run it yourself

1. A bot token from @BotFather and your Telegram user id.
2. An API token for **your** workspace: Settings → Подключения (or, on the host,
   `quests-server token add telegram <user> --workspace <id>`).
3. Anywhere with Python and access to the Quests URL:
   ```bash
   QUESTS_API=https://your-quests-host QUESTS_API_TOKEN=… QUESTS_TG_TOKEN=… QUESTS_TG_USER_IDS=<your id> \
     just run-telegram          # or: uv run quests-telegram
   ```
   If `api.telegram.org` is blocked where it runs, set `QUESTS_TG_PROXY`. Voice needs the STT sidecar
   (`just run-stt`, then `QUESTS_STT=http://127.0.0.1:8766`).

## On the operator's server

The compose services `bot` and `stt` are in the `bot` profile. With `COMPOSE_PROFILES=bot` in `.env` (set on the prod host)
they start as before; without it a plain `docker compose up -d --remove-orphans` stops and removes them — leave the variable in place
for as long as you want the bot.
