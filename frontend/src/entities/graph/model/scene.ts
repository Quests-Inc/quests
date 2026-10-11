// Как разложенная карта рисуется: рамки групп следуют за карточками, рёбра — за видимыми видами связей.
import { edgePath, spreadEnds, type EdgeItem } from './geometry.ts'
import type { GraphGroupView, GraphNodeView } from './types.ts'

type Node = GraphNodeView & { group?: string }
type Edge = { from: string; to: string; kind: string; draft?: string }

/** Рамка группы по её карточкам (у шара есть полоска заголовка сверху). Пустая группа остаётся как была. */
export function placedGroups<G extends GraphGroupView>(groups: G[], nodes: Node[]): G[] {
  return groups.map((g) => {
    const ms = nodes.filter((n) => n.group === g.key)
    if (!ms.length) return g
    const x0 = Math.min(...ms.map((n) => n.x - n.w / 2)) - 14
    const x1 = Math.max(...ms.map((n) => n.x + n.w / 2)) + 14
    const y0 = Math.min(...ms.map((n) => n.y - n.h / 2)) - 44
    const y1 = Math.max(...ms.map((n) => n.y + n.h / 2)) + 14
    return { ...g, x: (x0 + x1) / 2, y: (y0 + y1) / 2, w: x1 - x0, h: y1 - y0 }
  })
}

/**
 * Рёбра, которые реально рисуются. Шаг, который ждёт другой шаг, не рисует ещё и связь с квестом.
 * Связь владельца с карточкой внутри шара рисуется один раз — к самому шару.
 */
export function drawableEdges(input: {
  edges: Edge[]
  nodes: Node[]
  groups: GraphGroupView[]
  showEdge: Record<string, boolean>
}) {
  const nodeMap = new Map(input.nodes.map((n) => [n.key, n]))
  const groupMap = new Map(input.groups.map((g) => [g.key, g]))
  const chained = new Set(
    input.edges
      .filter((e) => e.kind === 'dependency' && e.from.startsWith('step:') && e.to.startsWith('step:'))
      .map((e) => e.to)
  )
  const items: EdgeItem<Edge>[] = []
  for (const e of input.edges) {
    if (!input.showEdge[e.kind]) continue
    const a = nodeMap.get(e.from)
    const b = nodeMap.get(e.to)
    if (!a || !b) continue
    if (e.kind === 'structure' && chained.has(e.to) && a.type === 'quest') continue
    if (e.kind === 'structure' && b.group && groupMap.get(b.group)?.owner === a.key) continue
    items.push({ e, a, b })
  }
  if (input.showEdge.structure) {
    for (const g of input.groups) {
      const owner = nodeMap.get(g.owner)
      if (owner) items.push({ e: { from: g.owner, to: g.key, kind: 'structure' }, a: owner, b: g })
    }
  }
  spreadEnds(items)
  return items.map((it) => ({ ...it.e, ...edgePath(it.a, it.b, it.oa ?? 0, it.ob ?? 0) }))
}

/** Куда ведёт «Открыть»: шаг открывает свой квест. Черновик никуда не открывается. */
export function stepOpenTarget(
  n: { draft?: unknown; type: string; id?: number | string; key: string } | null | undefined,
  data: { edges?: { to: string; from: string; kind: string }[]; nodes?: { key: string; id?: number | string }[] } | null
): [string, number] | null {
  if (!n || n.draft) return null
  if (n.type === 'quest' || n.type === 'note' || n.type === 'attachment')
    return typeof n.id === 'number' ? [n.type, n.id] : null
  if (n.type === 'step') {
    const e = data?.edges?.find((x) => x.to === n.key && x.kind === 'structure')
    const q = e && data?.nodes?.find((m) => m.key === e.from)
    return q && typeof q.id === 'number' ? ['quest', q.id] : null
  }
  return null
}
