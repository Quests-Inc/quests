// Группы вложений на карте: раскрыть блок, свернуть обратно на прежнее место, разложить на отдельные карточки.
// `saved` — позиции, выставленные руками; ключ `cluster:<владелец>` помнит, где стоял свёрнутый блок.
type Spot = { x: number; y: number }
type Box = { x: number; y: number; w: number; h: number; group?: string }

export function createClusterFolds(deps: {
  saved: Map<string, Spot>
  getNodes: () => Box[]
  getGroups: () => { key: string; type: string; owner: string; x: number; y: number }[]
}) {
  let expanded = $state(new Set<string>())
  /** Владельцы, чьи вложения разложены на отдельные карточки. */
  let loose = $state(new Set<string>())
  /** Где свёрнутая группа стояла до раскрытия: сворачиваясь, она возвращается туда. */
  const origins = new Map<string, Spot>()

  function membersCenter(g: { key: string; x: number; y: number }): Spot {
    const ms = deps.getNodes().filter((n) => n.group === g.key)
    if (!ms.length) return { x: g.x, y: g.y }
    return {
      x: (Math.min(...ms.map((m) => m.x - m.w / 2)) + Math.max(...ms.map((m) => m.x + m.w / 2))) / 2,
      y: (Math.min(...ms.map((m) => m.y - m.h / 2)) + Math.max(...ms.map((m) => m.y + m.h / 2))) / 2,
    }
  }

  /** Раскрытый блок мог сдвинуться, чтобы не наезжать на другие карточки: сворачиваясь, он встаёт на прежнее место. */
  function foldSpot(key: string) {
    const g = deps.getGroups().find((x) => x.type === 'attachment' && `cluster:${x.owner}` === key)
    const spot = origins.get(key) ?? (g ? membersCenter(g) : null)
    if (spot) deps.saved.set(key, spot)
    origins.delete(key)
  }

  function collapse(key: string) {
    foldSpot(key)
    expanded = new Set([...expanded].filter((k) => k !== key))
  }
  function collapseAll() {
    for (const k of expanded) foldSpot(k)
    expanded = new Set()
    loose = new Set()
  }
  /** Раскрыть группу (клик по свёрнутому узлу). */
  function expand(node: { key: string; x: number; y: number }) {
    deps.saved.set(node.key, { x: node.x, y: node.y })
    origins.set(node.key, { x: node.x, y: node.y })
    expanded = new Set([...expanded, node.key])
  }
  /** Разложить группу на отдельные карточки. */
  function disband(owner: string) {
    const key = `cluster:${owner}`
    if (expanded.has(key)) foldSpot(key)
    expanded = new Set([...expanded].filter((k) => k !== key))
    loose = new Set([...loose, owner])
  }
  /** Забыть всё (новый центр карты, очистка). */
  function reset() {
    expanded = new Set()
    loose = new Set()
  }
  /** Забыть запомненные места (но не раскрытые группы). */
  function forgetPositions() {
    deps.saved.clear()
    origins.clear()
  }

  return {
    get expanded() {
      return expanded
    },
    get loose() {
      return loose
    },
    collapse,
    collapseAll,
    expand,
    disband,
    reset,
    forgetPositions,
    /** Встать на сохранённый расклад: какие группы раскрыты и какие разложены. */
    restore(s: { expanded?: string[]; loose?: string[] }) {
      expanded = new Set(s.expanded ?? [])
      loose = new Set(s.loose ?? [])
      origins.clear()
    },
  }
}
