export type LayoutEntity = { key: string; title: string }
type EntityState = {
  entities?: LayoutEntity[]
  positions?: Record<string, unknown>
  hidden?: string[]
  center?: { type: string | null; id: number | null }
  roots?: { type: string; id: number }[]
}
const realKey = (key: string) => /^(quest|step|note|questline|attachment|template):[1-9]\d*$/.test(key)

/** Legacy layouts have no manifest: recover known cards from positions and roots. */
export function layoutEntities(state: EntityState | null | undefined): LayoutEntity[] {
  if (!state) return []
  const hidden = new Set(state.hidden ?? [])
  const entries = state.entities ?? [
    ...Object.keys(state.positions ?? {})
      .filter((key) => !hidden.has(key))
      .map((key) => ({ key, title: key })),
    ...[state.center, ...(state.roots ?? [])]
      .filter((r) => r?.type && r.id)
      .map((r) => ({ key: `${r!.type}:${r!.id}`, title: `${r!.type}:${r!.id}` })),
  ]
  return [...new Map(entries.filter((e) => realKey(e.key)).map((e) => [e.key, e])).values()].sort((a, b) =>
    a.key.localeCompare(b.key)
  )
}

export function diffLayoutEntities(before: EntityState | null | undefined, after: EntityState) {
  const old = new Map(layoutEntities(before).map((e) => [e.key, e]))
  const next = new Map(layoutEntities(after).map((e) => [e.key, e]))
  return {
    added: [...next.values()].filter((e) => !old.has(e.key)),
    removed: [...old.values()].filter((e) => !next.has(e.key)),
  }
}
