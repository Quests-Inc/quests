import type { EntitySearchHit, EntitySearchRequest } from '@/shared/api/entitySearch.ts'

export type CatalogTitle = { id: number; title: string }

function scopeKey(query: EntitySearchRequest) {
  return JSON.stringify({
    types: query.types,
    filters: query.filters ?? [],
    allowed: query.allowed ?? null,
    excluded: query.excluded ?? null,
  })
}

function plain(text: string) {
  return text.replaceAll('⟪', '').replaceAll('⟫', '')
}

function contains(hay: string, needle: string) {
  return hay.toLowerCase().includes(needle)
}

/** The new text continues the previous one inside the same filters and scope. */
export function queryExtends(previous: EntitySearchRequest, next: EntitySearchRequest) {
  return scopeKey(previous) === scopeKey(next) && next.q.startsWith(previous.q) && next.q.length > previous.q.length
}

/**
 * What to show while the server has not answered.
 * A longer query keeps previous rows whose title or snippet contains it, and adds note titles from the catalog.
 * A shorter query keeps the previous rows and still adds those titles. The server response replaces this draft.
 */
export function draftHits(
  previous: EntitySearchRequest | null,
  current: EntitySearchHit[] | null,
  query: EntitySearchRequest,
  catalog: CatalogTitle[] | null
): EntitySearchHit[] {
  let rows = current ? [...current] : []
  const needle = query.q.trim().toLowerCase()
  if (previous && needle && queryExtends(previous, query)) {
    rows = rows.filter((hit) => contains(hit.title || '', needle) || contains(plain(hit.snippet || ''), needle))
  }
  if (catalog && needle && query.types.includes('note')) {
    const seen = new Set(rows.filter((hit) => hit.kind === 'note').map((hit) => hit.id))
    for (const note of catalog) {
      if (seen.has(note.id) || !contains(note.title || '', needle)) continue
      rows.push({ kind: 'note', id: note.id, title: note.title, where: 'title', snippet: '', score: 0 })
    }
  }
  return rows
}
