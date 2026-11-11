# Look of the interface

How the site is laid out and painted, so a new screen looks like the old ones. Words are in [`naming.md`](naming.md); colours and
sizes are tokens in `frontend/src/app/styles/tokens.css`, never literals.

## General

1. **Tokens, not colours.** Surfaces `--color-bg` (page) → `--color-bg-raised` (header, modals, panels) → `--color-bg-muted`
   (controls); ink `--color-fg` / `-muted` / `-subtle`; accent `--color-accent` only for the main action and the current place.
2. **One surface, one depth.** A block that sits on a surface is cut from *that* surface, not from the page: see «Pills».
3. **Every state is visible**: hover, focus (`:focus-visible` outline), disabled, loading («Загрузка…»), empty (with a way out),
   error (`.modal__error` / toast, in Russian).
4. **Phones too**: 16px gutters, no horizontal scroll, targets at least 2rem.

## Pills: descriptions and explanations

Every description and every explaining paragraph is set into its surface as a **pill** — a darker rounded plate
(`--pill-bg`, `--pill-radius`, `--pill-shadow`), as the quest and step descriptions are.

- A plain hint (`.hint`) is only small subtle text, no plate. A paragraph that **explains** (`.explain`) is a pill.
- `--pill-bg` is derived from the surface the pill sits on. The page defines it from `--color-bg`; a modal / `.formscope`
  redefines it from `--color-bg-raised` (`modal.css`). A block with its own background (a card, a table, a panel) redefines
  `--pill-bg` the same way, from its own background, so the pill is always visibly darker than what is behind it. Never paste
  the page's pill colour onto another surface, and never give two layers the same colour.
- Danger / success / accent notes are the pill mixed with that colour (`color-mix(... var(--pill-bg))`), not a new background.

## Header

- Left: health, then the current workspace title with its colour/icon and a dropdown using the same switch operation as Settings. Workspace appearance is shared by members and editable by its owner in Settings → Workspace; built-in icons only. Middle: section tabs (labels give way to icons on narrow screens; groups open on hover). Right: actions.
- **Right-hand actions are icons only**, each with the same verb in `title` and `aria-label`: «Новый квест» (accent), «Команда»,
  «Окружение», «Теги», «Новый квестлайн», «Настройки».
- **All icons are drawn while there is room.** The row is measured (`ResizeObserver` on the grid cell); only when the next
  icon does not fit, the tail folds under «⋯» (always with that one button, so the menu itself takes one slot). No mixed
  state of labelled buttons plus «⋯»; a section that frees space gets its icons back.
- A new action takes a place in the priority list (`ACTIONS` in `JournalHeader.svelte`), most important first.

## Controls and forms

- Buttons: a verb; the main one is `.btn--accent`; an icon button has a hint. Destructive actions ask once and offer «Отменить»
  from the trash where they can.
- Labels above fields (`.label`), hints under them; sections of a form are named by what they hold.
- Modals: `ModalShell` + `ModalHead` + `ModalFoot`; Escape and the cross close; the first field has focus.
- Lists: a click opens, state is a dot / word with a hint, time is relative.

The workspace menu opens after 500 ms of mouse hover (click and keyboard open it immediately). Its footer contains “Добавить пространство” and a direct link to workspace settings. Owners can edit a workspace's name, built-in icon and colour from its pencil button or context click. The default runner uses the registered-device picker, retaining ordered fallbacks; settings action rows have an explicit gap and wrap on narrow screens.

## Standalone pages and user Markdown

A standalone tab is a page: it does not reuse `.modal` chrome, shadows, height limits or the close cross. Shared form
primitives live under `.formscope` as well as `.modal`. “Back” returns to the list; save/cancel actions remain reachable.
Templates have a searchable list, status and schedule filters, a result count, and an explicit row action menu. On narrow
containers the title and actions occupy separate rows. Disabled templates keep their errors readable and say “выключен”.

The pill rule above applies to UI descriptions/explanations. User Markdown quotes (`>`) are a distinct semantic block:
muted text, a left border, no rounded background. See [markdown.md](markdown.md) for syntax, math/PDF limits and examples.

Keep the present fonts, palette and character. Unify spacing, density, focus, empty/error states and action placement;
do not introduce a new visual identity as part of a cleanup. Layout must respond to container width, not only viewport
width: the same form may appear in a narrow pane on a wide screen. Grid children need `min-width: 0`, and footers wrap.


## Page layouts (2026-11-10)

Use `SidebarLayout` for journal, notes, scripts, graph and runs. Use `PageLayout` for hooks, attachments, history, templates and runners; calendar retains its dedicated layout. `PageLayout` owns the fixed top panel and scrollable work area. Do not repeat the active tab name as a page heading or add an introductory subtitle; retain titles for the specific entity being edited. Constrain readable forms inside the content, never centre the whole tab with an arbitrary maximum width. Internal split views remain page content.

Compose toolbar rows with `PageToolbar`. Search grows; choice groups retain natural width and wrap at group boundaries. Reuse `OptionPills` for short choices, `Picker` for a single searchable choice, `FilterSelect` for multiple choices, `DateInput` and `RunnerPicker` for their respective fields. Do not recreate page-specific segmented controls. Use `FilterChip` for removable source filters; `SidebarPicked` remains for bulk selection. Headers have no decorative icon or accent colour.

Pages own data, routing and domain actions; shared components own structure, responsive sizing and common interactions. Use Svelte snippets for composition rather than adding page-name switches. Modal chrome stays in modals; form primitives may be shared.

Background refresh must retain displayed data, scroll and expanded blocks. Show the loading placeholder only for the initial selection; discard stale responses and stop polling inactive tabs. Failed refreshes keep previous data and allow another attempt.

Toolbar controls belong in the PageLayout header, above the single separator. Do not put template search/filters inside modal/formscope content: this changes the field shape and spacing. Overview sections use flat rows and separators, not oversized raised cards. Search and create actions use the same shared field and icon-button sizing across tabs.

Runners use the shared search parser and FlagHints instead of a device picker: free text matches device labels, hostnames and job titles; `device=main-pc status=queued` (or `:`) narrows both devices and jobs. Negative tokens exclude, repeated values are alternatives. A job status selects devices with matching jobs; online/offline selects jobs through their device. Section headings have a subtle tonal strip and space between sections; rows remain flat. Filtered empty states must describe no matches, not claim that jobs never existed.

Runner content is grouped by device, with device presence and management in the group header and status badges in each job row. No separate device, running, queue or recent sections. Active jobs sort first; repeated finished checks group by source within the same device and expose their history. Missing devices retain a named group. Device registration lives behind the toolbar add action.

Runner toolbar has two rows: search/add above, the view slider below. Search, add button and slider share a 2.25rem height. The view slider reads/writes `view=active|recent` in the same search string; selecting all removes that filter while preserving other text and filters.
