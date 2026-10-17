// Where each open note was scrolled to: coming back to a note (or to the tab) puts the page where it was.
const MAX = 60
const memory = new Map<number, number>()

export function rememberScroll(id: number, top: number) {
  memory.delete(id)
  if (top > 0) memory.set(id, top)
  if (memory.size > MAX) memory.delete(memory.keys().next().value as number)
}

export function recalledScroll(id: number): number | null {
  return memory.get(id) ?? null
}

/** Ties the memory to a note page: `onScroll` records, `sync` (call it inside an `$effect`) puts the position back once the note is shown. */
export function createNoteScroll(deps: {
  shownId: () => number | null
  selectedId: () => number | null
  page: () => HTMLElement | null
}) {
  let restoredFor: number | null = null
  return {
    onScroll() {
      const id = deps.shownId()
      const page = deps.page()
      if (id != null && id === deps.selectedId() && page) rememberScroll(id, page.scrollTop)
    },
    sync() {
      const id = deps.shownId()
      if (id == null || id !== deps.selectedId()) {
        restoredFor = null
        return
      }
      if (restoredFor === id) return
      restoredFor = id
      const top = recalledScroll(id)
      if (top == null) return
      requestAnimationFrame(() => {
        const page = deps.page()
        if (page) page.scrollTop = top
      })
    },
  }
}

/**
 * Scrolls only the tree list to the open row. scrollIntoView would also scroll the tab
 * stage horizontally while the page is still sliding in from the right, making the notes jerk sideways.
 */
export function revealOpenRow(root: ParentNode = document) {
  const row = root.querySelector<HTMLElement>('.notes__row--on')
  const list = row?.closest<HTMLElement>('.notes__list')
  if (!row || !list) return
  const r = row.getBoundingClientRect()
  const l = list.getBoundingClientRect()
  if (r.top < l.top) list.scrollTop += r.top - l.top
  else if (r.bottom > l.bottom) list.scrollTop += r.bottom - l.bottom
}
