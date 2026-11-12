/** Exact IDs are identity matches, never fuzzy substrings of an ID. */
export function searchIdentity(query: string): { kind: string; id: string } | null {
  const m = /^(?:([a-z][a-z_-]*)\s*[=:]\s*)?([1-9]\d*)$/i.exec(query.trim())
  return m ? { kind: (m[1] ?? '').toLowerCase(), id: m[2] } : null
}

export function identityMatches(query: string, kind: string, id: string | number): boolean {
  const target = searchIdentity(query)
  return !!target && (!target.kind || target.kind === kind) && target.id === String(id)
}

/** A typed reference is a strict identity query; bare numbers also retain ordinary text matches. */
export function entityMatches(query: string, kind: string, id: string | number, text: string): boolean {
  const q = query.trim().toLowerCase()
  const target = searchIdentity(q)
  if (target?.kind) return identityMatches(q, kind, id)
  return identityMatches(q, kind, id) || text.toLowerCase().includes(q)
}

/** Stable ranking before slicing/limiting a mixed list of matches. */
export function rankIdentity<T>(
  items: T[],
  query: string,
  identity: (item: T) => { kind: string; id: string | number }
): T[] {
  return [...items].sort((a, b) => {
    const x = identity(a),
      y = identity(b)
    return Number(identityMatches(query, y.kind, y.id)) - Number(identityMatches(query, x.kind, x.id))
  })
}
