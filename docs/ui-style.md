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

- Left: health. Middle: section tabs (labels give way to icons on narrow screens; groups open on hover). Right: actions.
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
