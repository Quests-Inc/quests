// Какие карточки остаются на карте после черновика: скрытые, чужие статусы, оторванные от центров, выключенные виды.
// Чистая функция — страница только подставляет текущие фильтры.
import { ENTITY_TYPES } from './constants.ts'

type Node = { key: string; type: string; draft?: unknown; status?: string }
type Edge = { from: string; to: string; kind: string }

/**
 * Скрытые карточки уходят вместе с рёбрами (центр — никогда). Квесты вне выбранных статусов уводят за собой
 * шаги и вложения, которые висят только на них. Что потеряло путь до каждого центра, тоже уходит
 * (черновик остаётся). Виды, выключенные в фильтре, снимаются последними — чтобы не пропало то,
 * до чего добираются только через них.
 */
export function visibleGraph<N extends Node, E extends Edge>(input: {
  root: string
  nodes: N[]
  edges: E[]
  draftRoots: { type: string; id: number }[]
  hidden: Set<string>
  questStatuses: Set<string>
  shownTypes: Set<string>
}): { nodes: N[]; edges: E[]; filteredOut: number } {
  const rootKeys = new Set([input.root, ...input.draftRoots.map((r) => `${r.type}:${r.id}`)])
  let nodes = input.nodes.filter((n) => !input.hidden.has(n.key) || n.key === input.root)
  const offQuests = new Set(
    nodes
      .filter((n) => n.type === 'quest' && !n.draft && !rootKeys.has(n.key) && !input.questStatuses.has(n.status ?? ''))
      .map((n) => n.key)
  )
  if (offQuests.size) {
    const withThem = new Set(offQuests)
    for (const e of input.edges) {
      if (e.kind === 'structure' && offQuests.has(e.from) && /^(step|attachment):/.test(e.to)) withThem.add(e.to)
    }
    nodes = nodes.filter((n) => !withThem.has(n.key))
  }
  const filteredOut = offQuests.size
  {
    const adj = new Map<string, string[]>()
    const link = (a: string, b: string) => {
      const list = adj.get(a)
      if (list) list.push(b)
      else adj.set(a, [b])
    }
    const present = new Set(nodes.map((n) => n.key))
    for (const e of input.edges) {
      if (present.has(e.from) && present.has(e.to)) {
        link(e.from, e.to)
        link(e.to, e.from)
      }
    }
    const reach = new Set([...rootKeys].filter((k) => present.has(k)))
    for (const k of reach) for (const nb of adj.get(k) ?? []) reach.add(nb)
    if (reach.size) nodes = nodes.filter((n) => n.draft || reach.has(n.key))
  }
  if (input.shownTypes.size < ENTITY_TYPES.length) {
    const off = new Set(
      nodes
        .filter(
          (n) => !(n.draft || n.key === input.root || input.shownTypes.has(n.type) || !ENTITY_TYPES.includes(n.type))
        )
        .map((n) => n.key)
    )
    const withThem = new Set(off)
    for (const e of input.edges) {
      if (e.kind === 'structure' && off.has(e.from) && /^(step|attachment):/.test(e.to)) withThem.add(e.to)
    }
    nodes = nodes.filter((n) => n.draft || n.key === input.root || !withThem.has(n.key))
  }
  const keep = new Set(nodes.map((n) => n.key))
  return { nodes, edges: input.edges.filter((e) => keep.has(e.from) && keep.has(e.to)), filteredOut }
}

/** Скрытые карточки по соседу, который ещё на данных карты: ключ соседа → вид → ключи. */
export function hiddenNeighbors(
  edges: { from: string; to: string }[],
  hidden: Set<string>
): Map<string, Record<string, string[]>> {
  const out = new Map<string, Record<string, string[]>>()
  if (!hidden.size) return out
  const add = (from: string, other: string) => {
    if (!hidden.has(other) || hidden.has(from)) return
    const type = other.slice(0, other.indexOf(':'))
    let by = out.get(from)
    if (!by) {
      by = {}
      out.set(from, by)
    }
    if (!by[type]) by[type] = []
    if (!by[type].includes(other)) by[type].push(other)
  }
  for (const e of edges) {
    add(e.from, e.to)
    add(e.to, e.from)
  }
  return out
}

/** Кружки нераскрытых связей на карточке: что карта ещё не дотянула и что сняли вручную. */
export function borderMarks(
  node: { draft?: unknown; key: string; more?: Record<string, string | string[] | undefined> },
  hiddenAdj: Map<string, Record<string, string[]>>,
  shownTypes: Set<string>,
  statusOff: (key: string) => boolean
): { type: string; count: number }[] {
  if (node.draft) return []
  const own = node.more ?? {}
  const gone = hiddenAdj.get(node.key) ?? {}
  const out: { type: string; count: number }[] = []
  for (const type of ENTITY_TYPES) {
    if (!shownTypes.has(type)) continue
    const ownKeys = own[type]
    const keys = [...(Array.isArray(ownKeys) ? ownKeys : []), ...(gone[type] ?? [])]
    const count = keys.filter((k) => !statusOff(k)).length
    if (count) out.push({ type, count })
  }
  return out
}
