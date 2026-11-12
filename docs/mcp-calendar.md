# Calendar events over MCP

The `event` tool manages calendar events. Automation signals still belong to `hook`; they are different concepts.

- `action=list`: compact pages; optional `q` uses the shared ID/text search, and `cursor` continues a page.
- `action=get, id=N`: full fields, with `max_chars` / `offset` for a long description.
- `action=create`: title, starts_at and ends_at are required; optional description, all_day, color, built-in icon.
- `action=update, id=N`: only supplied fields change.
- `action=delete, id=N`: removes the event permanently. Linked quests keep the last resolved instant as a fixed deadline, with their urgency window and expiry policy.

Dates use RFC3339 with a timezone and are normalized to UTC by the existing API. Use `icon(action=set|clear|color,event=N,…)` for custom images/colour.

`get_quest` includes `event_id`, `event_title`, `event_edge`, `event_dir`, `event_offset_seconds` and `expire_on_deadline`. The resolved `deadline_at` alone is not a complete description of a relative deadline.

```json
{"title":"Prepare slides","questline":"none","event_id":12,"event_edge":"start","event_dir":"before","event_offset_seconds":86400,"duration_seconds":7200,"expire_on_deadline":true}
```

That quest is due one day before event 12 starts, becomes urgent two hours before due, and expires when due. Moving the event updates the due instant while retaining the relation and options.

`update_quest` accepts the same fields; a single phrase field can change without resending all the others. `clear_event=true` detaches the relation and keeps the resolved date (or accepts a new fixed `deadline_at`). `clear_deadline=true` removes either source. A simultaneous event ID and fixed instant is rejected as ambiguous.

Template MCP fields also include the independent expiry policy. `search_entities` exposes the same cross-entity search contract used by the web: `q`, `types`, filters/scopes, limit and offset; results include `more` and `next`. MCP privacy and workspace isolation are enforced by the HTTP API, not by trusting arguments sent by the agent.
