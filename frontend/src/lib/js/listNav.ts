/**
 * Keyboard walk through a list that is already on screen (design: note=76).
 *
 * The list marks its rows with data-nav and its container with data-nav-root.
 * "Next row" is the next marked element in DOM order, so the walk always
 * matches what is visible: a collapsed group is unmounted and drops out by
 * itself. The cursor is the browser focus, which also makes Enter / Space do
 * what a click on that row does.
 */

function visibleRows() {
  // Several lists can be mounted at once (the journal sidebar stays in the DOM
  // under the notes tab, hidden): take the one that is actually on screen.
  for (const root of document.querySelectorAll('[data-nav-root]')) {
    const rows = [...root.querySelectorAll('[data-nav]')].filter(
      (el) => el instanceof HTMLElement && el.getClientRects().length > 0
    )
    if (rows.length) return rows
  }
  return []
}

/** The row to start from when focus is not in the list: the selected one, else null. */
function selectedRow(rows: any[]) {
  return (
    rows.find(
      (el: { classList: { contains: (arg0: string) => any } }) =>
        el.classList.contains('quest-row--active') || el.classList.contains('notes__row--on')
    ) ?? null
  )
}

/**
 * Move focus to the next (+1) or previous (-1) row. Returns false when the
 * page has no list, so the key keeps its default behaviour.
 */
export function moveRow(dir: number) {
  const rows = visibleRows()
  if (rows.length === 0) return false
  const at = rows.indexOf(/** @type {HTMLElement} */ (document.activeElement))
  let next
  if (at !== -1) {
    next = rows[Math.min(rows.length - 1, Math.max(0, at + dir))]
  } else {
    const sel = selectedRow(rows)
    // First press lands on the selected row itself; after that it walks.
    next = sel ?? rows[dir > 0 ? 0 : rows.length - 1]
  }
  next.focus({ preventScroll: true })
  next.scrollIntoView({ block: 'nearest' })
  return true
}

/** The button that folds the group/node the focused row belongs to. */
function foldButton(el: HTMLElement) {
  if (el.hasAttribute('aria-expanded')) return el
  return el.closest('[data-nav-node]')?.querySelector('[aria-expanded]') ?? null
}

/**
 * Expand (open=true) or collapse (open=false) the focused row's group.
 * Returns false when there is nothing to fold or it is already in that state.
 */
export function foldRow(open: boolean) {
  const el = document.activeElement
  if (!(el instanceof HTMLElement) || !el.closest('[data-nav-root]')) return false
  const btn = foldButton(el)
  if (!(btn instanceof HTMLElement)) return false
  if ((btn.getAttribute('aria-expanded') === 'true') === open) return false
  btn.click()
  return true
}
