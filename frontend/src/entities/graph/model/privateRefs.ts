import { parseRefs } from '@/shared/lib/refs.ts'

/** Reference edges withheld with a private note. Only towards nodes already on the map. */
export function referenceEdges(
  nodes: { key: string }[],
  edges: { from: string; to: string; kind: string }[],
  pairs: [string, string][]
) {
  const keys = new Set(nodes.map((n) => n.key))
  const have = new Set(edges.map((e) => `${e.from}|${e.to}|${e.kind}`))
  const extra: { from: string; to: string; kind: string }[] = []
  for (const [from, text] of pairs) {
    if (!keys.has(from)) continue
    for (const ref of parseRefs(text)) {
      const to = `${ref.kind}:${ref.id}`
      const id = `${from}|${to}|reference`
      if (!keys.has(to) || to === from || have.has(id)) continue
      have.add(id)
      extra.push({ from, to, kind: 'reference' })
    }
  }
  return extra
}
