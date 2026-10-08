# Connecting your own clients (HUD, MCP, Android, CLI)

Every client talks to the same API with a **bearer token that belongs to one workspace**. You mint it yourself:
**Settings → Подключения → Новый токен**, name it after the device or program, copy the secret (shown once) and the two lines it
offers:

```
QUESTS_API=https://<your-quests-address>
QUESTS_API_TOKEN=<secret>
```

A token works in the workspace you were in when you created it and sees only that workspace. To use another workspace from a
client, switch to it in the web interface and mint a separate token there. Revoke a token in the same place; the client stops at once.

| Client | Where the two values go |
|---|---|
| **HUD** (desktop overlay) | `~/.config/quests/overlay.env`, see [hud-workstation.md](hud-workstation.md) (step 1 there can be skipped: use the token from the interface) |
| **MCP / agents** | environment of `quests-mcp` (`uv run quests-mcp`), e.g. in the MCP server config of your agent |
| **Android HUD** | app settings: server address and token |
| **CLI** | `export QUESTS_API=… QUESTS_API_TOKEN=…`, then `quests list`; `QUESTS_WORKSPACE=<id>` picks a workspace if the token allows it |
| **Telegram bot** | optional personal client, see [telegram-bot.md](telegram-bot.md) |

Notes
- Only a signed-in browser session creates or revokes tokens; a token cannot manage tokens (a leaked one stays contained).
- At most 25 live tokens per account.
- Check a token: `curl -H "Authorization: Bearer $QUESTS_API_TOKEN" $QUESTS_API/api/workspaces` shows the workspace it is bound to
  (`current`).
- Operators can also mint tokens on the host: `quests-server token add <name> [user] --workspace <id>`.
