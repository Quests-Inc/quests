# Deadlines: source, urgency and expiry

A quest deadline has three independent choices:

1. **Source**: a fixed UTC instant, or an event's start/end plus a before/after offset. `deadline_at` stores the resolved instant; `event_id`, `event_edge`, `event_dir`, `event_offset_seconds` retain the live relation.
2. **Urgency window**: `duration_seconds`, a nonnegative number of seconds before the resolved instant. Zero means no urgency window, regardless of source.
3. **Expiry policy**: `expire_on_deadline`. When true, an unfinished active quest becomes `expired` after its deadline. When false, reaching the date leaves it active. Window length does not enable/disable expiry.

Both source forms show the same window and expiry controls. A source switch does not clear either setting. Event offsets and urgency windows have different jobs: “two days before the conference” determines the due instant; “start reminding one day before due” determines urgency. An event's own duration is neither of those durations.

| Example | Source | Window | Expiry |
|---|---|---|---|
| Deliver by Friday without prior reminders | Fixed Friday | None | On |
| Prepare slides for a meeting | Before event start | Explicit preparation window | On |
| Follow up an hour after a meeting ends | After event end + 1 hour | Optional | On |
| An approximate planning milestone | Fixed or relative | Optional | Off |
| Start thinking about a long-range task | Fixed or relative | Explicit reminder window | Off |
| A multi-day event | Choose start or end deliberately | Independent of event length | Either |

The default for new quests made through the HTTP API/editor is expiry on; an omitted window is zero. Existing quests and templates are migrated to preserve their former behaviour. Existing template NULL durations retain their legacy midnight-based materialization; newly edited fixed-time templates write zero explicitly for an empty window. Internal legacy constructors can omit the new pointer and retain the former positive-window policy at the storage boundary; persisted reads always carry an explicit value.

## Presentation and boundaries

The server and desktop HUD share `platform/timeutil.DeadlineAt`. The web recomputes the same thresholds as the clock advances, rather than holding an obsolete `timer_tone` snapshot.

- Future, no window: neutral countdown. No synthetic seven-day window.
- Positive window: remaining/window above 2/3 is green, above 1/3 orange, otherwise red. Before the window opens the countdown is green but not urgent.
- The urgency interval is `[deadline-window, deadline)`, including its start. Closed/frozen quests are not scheduled for urgency notifications.
- At/after the date, expiry on: red “просрочено”, and the scheduler changes active → expired.
- At/after the date, expiry off: neutral “срок наступил”, no urgency and no automatic status change.
- Completed/failed/archived/frozen quests do not show a web countdown. Reaching a date never completes or fails a quest automatically.

## Editing and event lifecycle

Moving an event and updating linked dates is one transaction. The window and expiry policy survive. A previously expired quest whose new due date is future becomes active; completed, failed and frozen quests retain their status. Removing an event keeps the last resolved instant as a fixed deadline and preserves the options.

A PATCH may change one part of a live event phrase without resending the event ID. Setting a fixed `deadline_at` on an event-bound quest requires clearing `event_id`, so a cached instant is not mistaken for an independent source. Calendar-event PATCH really changes only supplied fields. Dates use the existing UTC API / local-zone editor conversion; a before/after offset is elapsed seconds, including across DST.

The calendar’s all-day flag changes presentation, not the rule: callers still choose the start or end instant. Recurring event occurrences, independent per-event reminder schedules, and “business days before” offsets are outside this model; they need explicit separate semantics rather than reusing `duration_seconds`.
