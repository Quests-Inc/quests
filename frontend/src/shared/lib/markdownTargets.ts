/** Versioned opt-in envelope; ordinary script strings (including JSON) stay unchanged. */
export type MarkdownTarget = { table: string; row: string }
export type TargetResult = { text: string; target: MarkdownTarget }
const KEY = /^[a-z][a-z0-9_-]{0,63}$/
export function targetResult(raw: string): TargetResult | null {
  if (!raw.trimStart().startsWith('{') || raw.length > 20000) return null
  try {
    const v = JSON.parse(raw)
    if (
      v.kind !== 'markdown-target' ||
      v.version !== 1 ||
      typeof v.text !== 'string' ||
      !KEY.test(v.target?.table) ||
      !KEY.test(v.target?.row)
    )
      return null
    return { text: v.text, target: { table: v.target.table, row: v.target.row } }
  } catch {
    return null
  }
}

/** Resolve only within the rendered document, never into another note or raw CSS selectors. */
export function findMarkdownTarget(root: HTMLElement, target: MarkdownTarget): HTMLElement | null {
  const tables = [...root.querySelectorAll<HTMLElement>('table[data-md-table]')].filter(
    (t) => t.dataset.mdTable === target.table
  )
  if (tables.length !== 1) return null
  const rows = [...tables[0].querySelectorAll<HTMLElement>('tr[data-md-row]')].filter(
    (r) => r.dataset.mdRow === target.row
  )
  return rows.length === 1 ? rows[0] : null
}

/** Metadata comments are consumed before sanitizing, and never become visible text. */
export function markTableTargets(dirty: string): string {
  if (!dirty.includes('<table')) return dirty
  const doc = window.document.implementation.createHTMLDocument('')
  doc.body.innerHTML = dirty
  for (const table of doc.querySelectorAll<HTMLTableElement>('table')) {
    let prev = table.previousSibling
    while (prev && (prev.nodeType === 8 || (prev.nodeType === 3 && !prev.textContent?.trim()))) {
      const next = prev.previousSibling
      if (prev.nodeType === 8) {
        const match = /^\s*table-id:\s*([a-z][a-z0-9_-]{0,63})\s*$/.exec(prev.textContent ?? '')
        if (match) {
          table.dataset.mdTable = match[1]
          prev.remove()
        }
      }
      prev = next
    }
    for (const row of table.rows) {
      // Markers live in cells, so adding/removing/reordering rows does not change their identity.
      const walk = doc.createTreeWalker(row, 128)
      const comments: Comment[] = []
      while (walk.nextNode()) comments.push(walk.currentNode as Comment)
      for (const c of comments) {
        const match = /^\s*row-id:\s*([a-z][a-z0-9_-]{0,63})\s*$/.exec(c.data)
        if (match) {
          row.dataset.mdRow = match[1]
          c.remove()
        }
      }
    }
  }
  return doc.body.innerHTML
}
