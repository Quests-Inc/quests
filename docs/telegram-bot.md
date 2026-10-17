# Telegram bot (optional client)

The bot is **not part of the platform**: it is an ordinary API client, like the HUD or the CLI. It talks to Quests with one
API token, so it works in exactly one workspace, for the Telegram ids you whitelist. The hosted multi-user service does not run
one for others; run it yourself, on your own hardware, against your own account.

What it gives you: push messages for quest events and deadline windows, and commands and the action assistant from a chat.

Voice input (a `faster-whisper` sidecar, `quests-stt`) was removed on 2026-10-17: the host could not run it comfortably and its
image was ~1.4 GB on every release. If it comes back, prefer a hosted Whisper API (Groq) over a local sidecar.

## Run it yourself

1. A bot token from @BotFather and your Telegram user id.
2. An API token for **your** workspace: Settings → Подключения (or, on the host,
   `quests-server token add telegram <user> --workspace <id>`).
3. Anywhere with Python and access to the Quests URL:
   ```bash
   QUESTS_API=https://your-quests-host QUESTS_API_TOKEN=… QUESTS_TG_TOKEN=… QUESTS_TG_USER_IDS=<your id> \
     just run-telegram          # or: uv run quests-telegram
   ```
   If `api.telegram.org` is blocked where it runs, set `QUESTS_TG_PROXY`.

## On the host admin's server

The compose service `bot` is in the `bot` profile. On prod the profile is switched on in `.doco-cd.yml` of the deploy
repository (`Quests-Inc/quests-deploy`, `profiles: [bot]`); remove it there and the next rollout stops the bot — keep it for as long as you want the bot.
