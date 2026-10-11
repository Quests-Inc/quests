/** Session cache of attachment *metadata* (not file bytes).
 *
 * Journal load seeds every owner from GET /api/attachments (SQLite only).
 * Opening a quest then paints instantly; WebDAV Stat runs once per owner
 * in the background so `available` / `source_updated` can fill in later.
 */

function key(ownerType: string, ownerId: string | number) {
  return `${ownerType}:${ownerId}`
}

function asIdList(rows: any) {
  return (rows || []).map((row: { id: any }) => row.id).join(',')
}

const cache = new Map<string, { items: any[]; probed: boolean }>()
const liveListeners = new Set<() => void>()

function notifyLive() {
  for (const fn of liveListeners) fn()
}

export function peekAttachmentList(ownerType: string, ownerId: string | number) {
  if (ownerId == null || ownerId === '') return null
  return cache.get(key(ownerType, ownerId)) ?? null
}

export function setAttachmentList(ownerType: string, ownerId: string | number, items: any[], { probed = false } = {}) {
  cache.set(key(ownerType, ownerId), { items: items || [], probed })
}

function rowsFor(bucket: { [x: string]: any }, id: string | number) {
  if (!bucket || typeof bucket !== 'object') return []
  return bucket[id] || bucket[String(id)] || []
}

function keepLiveFlags(prevItems: any[] | undefined, incoming: any[] | undefined) {
  const prev = new Map<number, any>((prevItems || []).map((row) => [row.id, row]))
  return (incoming || []).map((row) => {
    const old = prev.get(row.id)
    if (!old) return row
    return {
      ...row,
      available: old.available,
      source_updated: old.source_updated,
      last_modified: old.last_modified,
    }
  })
}

/** Seed (or refresh) every listed owner. Empty owners get `[]` so a click
 * does not look like a cache miss and refetch. */
export function seedAttachmentIndex(
  index: Record<string, Record<string, any[]>>,
  { questIds = [], questlineIds = [], noteIds = [] } = {}
) {
  const byQuest = index?.quest ?? {}
  const byLine = index?.questline ?? {}
  const byNote = index?.note ?? {}
  const apply = (type: string, id: any, incoming: any) => {
    const k = key(type, id)
    const cur = cache.get(k)
    if (cur?.probed && asIdList(cur.items) === asIdList(incoming)) {
      cache.set(k, { probed: true, items: keepLiveFlags(cur.items, incoming) })
      return
    }
    cache.set(k, { items: incoming, probed: incoming.length === 0 })
  }
  for (const id of questIds) apply('quest', id, rowsFor(byQuest, id))
  for (const id of questlineIds) apply('questline', id, rowsFor(byLine, id))
  for (const id of noteIds) apply('note', id, rowsFor(byNote, id))
}

/** Drop live WebDAV flags so the open quest re-STATs after DAV flips. */
export function invalidateAttachmentLiveFlags() {
  for (const [k, entry] of cache) {
    if (entry?.items?.length) {
      cache.set(k, { items: entry.items, probed: false })
    }
  }
  notifyLive()
}

export function onAttachmentLiveInvalidate(fn: () => void) {
  liveListeners.add(fn)
  return () => liveListeners.delete(fn)
}

/** Flat list from GET /api/attachments grouped payload. */
export function flattenAttachmentIndex(index: any) {
  if (!index || typeof index !== 'object') return []
  const out: any[] = []
  for (const type of ['quest', 'questline', 'note']) {
    const bucket = index[type]
    if (!bucket || typeof bucket !== 'object') continue
    for (const rows of Object.values<any[]>(bucket)) {
      for (const a of rows || []) {
        out.push({
          ...a,
          owner_type: a.owner_type || type,
          owner_id: a.owner_id ?? Number(a.owner_id),
        })
      }
    }
  }
  return out
}
