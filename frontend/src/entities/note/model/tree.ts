// Дерево заметок: фильтры поиска, порядок, родословная. Чистые функции над списком заметок.

/** Filters typed into the search box: `flag:name` (`-flag:name` = without it). */
export const NOTE_SPEC = {
  flag: {
    label: 'флаг',
    all: true,
    values: [
      { id: 'outdated', label: 'устарела', aliases: ['stale'] },
      { id: 'readme', label: 'README' },
      { id: 'private', label: 'PRIVATE' },
      { id: 'category', label: 'CATEGORY' },
      { id: 'pinned', label: 'закреплена', aliases: ['pin'] },
      { id: 'check', label: 'есть проверка актуальности' },
    ],
    test: (
      n: { outdated: any; is_readme: any; is_private: any; is_category: any; pinned: any; has_check: any },
      v: string | number
    ) =>
      ({
        outdated: !!n.outdated,
        readme: !!n.is_readme,
        private: !!n.is_private,
        category: !!n.is_category,
        pinned: !!n.pinned,
        check: !!n.has_check,
      })[v],
  },
}

/** The given notes and the chain of their parents, in the original order. */
export function withAncestors(notes: any[], list: any[]) {
  const byId = new Map(notes.map((n) => [n.id, n]))
  const keep = new Set()
  for (const n of list) {
    let cur = n
    keep.add(cur.id)
    while (cur.parent_id != null) {
      const parent = byId.get(cur.parent_id)
      if (!parent) break
      keep.add(parent.id)
      cur = parent
    }
  }
  return notes.filter((n) => keep.has(n.id))
}

/** Tree order: with children → pinned → sort_order → id. */
export function compareNotes(a: any, b: any, hasKids: (n: any) => boolean) {
  return (
    Number(hasKids(b)) - Number(hasKids(a)) ||
    Number(b.pinned) - Number(a.pinned) ||
    (a.sort_order || 0) - (b.sort_order || 0) ||
    a.id - b.id
  )
}

/** candidate is under ancestor (cannot become its parent). */
export function isUnder(notes: any[], candidateId: number, ancestorId: number) {
  const noteById = (id: number) => notes.find((n) => n.id === id) ?? null
  if (candidateId == null || ancestorId == null) return false
  let cur = noteById(candidateId)
  const seen = new Set()
  while (cur?.parent_id != null && !seen.has(cur.id)) {
    seen.add(cur.id)
    if (cur.parent_id === ancestorId) return true
    cur = noteById(cur.parent_id)
  }
  return false
}

/** Ids of all ancestors of a note, nearest first (cycle-safe). */
export function ancestorIds(notes: any[], id: number): number[] {
  const byId = new Map(notes.map((n) => [n.id, n]))
  const out: number[] = []
  for (
    let cur = byId.get(id);
    cur && cur.parent_id != null && !out.includes(cur.parent_id);
    cur = byId.get(cur.parent_id)
  ) {
    out.push(cur.parent_id)
  }
  return out
}
