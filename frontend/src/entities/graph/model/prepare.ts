import { CLUSTER_MIN } from './constants.ts'

/** Collapses bulky attachment lists into one cluster node. */
export function prepareGraph(g: { root?: any; nodes?: any; edges?: any }, open_: Set<string>, loose: Set<string>) {
  const nodes = g.nodes.map((n: any) => ({ ...n, x: 0, y: 0, w: 0, h: 0 }))
  const edges = g.edges.map((e: any) => ({ ...e }))
  const byKey = new Map<string, any>(nodes.map((n: { key: any }): [string, any] => [n.key, n]))
  const owners = new Map<string, any[]>()
  for (const e of edges) {
    const to = byKey.get(e.to)
    if (e.kind === 'structure' && to?.type === 'attachment' && to.links === 0 && !to.draft && !e.draft) {
      if (!owners.has(e.from)) owners.set(e.from, [])
      owners.get(e.from).push(to.key)
    }
  }
  const drop = new Set()
  for (const [owner, keys] of owners) {
    const ck = `cluster:${owner}`
    if (keys.length < CLUSTER_MIN || open_.has(ck) || loose.has(owner)) continue
    for (const k of keys) drop.add(k)
    nodes.push({
      key: ck,
      type: 'cluster',
      id: 0,
      title: `${keys.length} вложений`,
      depth: (byKey.get(owner)?.depth ?? 0) + 1,
      links: 0,
      x: 0,
      y: 0,
      w: 0,
      h: 0,
    })
    edges.push({ from: owner, to: ck, kind: 'structure' })
  }
  return {
    nodes: nodes.filter((n: { key: unknown }) => !drop.has(n.key)),
    edges: edges.filter((e: { from: unknown; to: unknown }) => !drop.has(e.from) && !drop.has(e.to)),
  }
}
