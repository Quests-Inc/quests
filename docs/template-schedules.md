# Template schedules

Creating and editing a template opens a modal over the template list. The list stays visible behind it; the dialog header and footer stay pinned while its body scrolls. Row action buttons have no persistent background, and the filtered/total count is aligned right.

The period selector has four modes. API values `daily` and `weekly` are unchanged for existing templates.

| UI | freq | Fields | Meaning |
|---|---|---|---|
| День | daily | — | Every local calendar day |
| Неделя | weekly | weekdays | Comma-separated weekdays, Monday=0 through Sunday=6 |
| Месяц | monthly | monthdays | Comma-separated dates 1–31; dates absent from the current month are skipped |
| Свой промежуток | cycle | cycle_length, cycle_start, cycle_days | Repeating calendar-day cycle anchored at day 1 |

`cycle_length` is an integer from 1 to 366. `cycle_start` is YYYY-MM-DD in the template timezone. `cycle_days` contains 1-based emitting days within that cycle. No automatic emission happens before the anchor. For work 2/2, use length 4, days `1,2`; the anchor is the first working day. Calendar dates, rather than elapsed 24-hour durations, keep the phase stable through DST. Monthly `15` emits on the 15th; `30` does nothing during February and never rolls over to March.

At least one day must be selected for monthly/cycle schedules. API validation also applies to updates and copies. Inactive period settings are retained when switching modes. Migration 0044 adds fields with defaults without changing existing daily/weekly behavior.

The period selects eligible dates; fixed/surprise timing, runner/script handling, daily deduplication, deadlines, chance and urgency retain their existing behavior. Missed past dates are not backfilled. Changing a period does not recreate a quest already emitted that day. Manual “emit now” still bypasses the schedule. MCP create/update/read/list and copy support all four modes.
