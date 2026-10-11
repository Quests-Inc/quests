// Связи на карте: что можно выбрать вторым концом при создании ссылки, кто чей потомок, какие ссылки уже есть.
// Чистые функции над рёбрами карты (черновик уже наложен) и таблицей узлов.

export interface LinkEdge {
  from: string
  to: string
  kind: string
  draft?: string
  op?: number
}

type NodeLike = { key: string; type: string }

export type LinkMode = 'to' | 'from' | 'attach'

export const REF_FROM = new Set(['quest', 'note', 'step', 'questline'])
const REF_TO = new Set(['quest', 'note', 'step', 'attachment', 'questline'])

/** Keys of the notes below this one, over the structure edges on the map (draft included). */
export function noteDescendantKeys(edges: LinkEdge[], key: string): Set<string> {
  const seen = new Set<string>()
  const walk = (k: string) => {
    for (const e of edges) {
      if (
        e.kind === 'structure' &&
        e.from === k &&
        e.draft !== 'remove' &&
        e.to.startsWith('note:') &&
        !seen.has(e.to)
      ) {
        seen.add(e.to)
        walk(e.to)
      }
    }
  }
  walk(key)
  return seen
}

/** Whether the card can be picked as the other end in the current link mode. */
export function canPickTarget(
  linking: { mode: LinkMode; anchor: string } | null,
  n: { key: string; type?: string } | null,
  nodes: Map<string, NodeLike>,
  edges: LinkEdge[]
): boolean {
  if (!linking || !n || n.key === linking.anchor || n.type === 'cluster') return false
  const a = nodes.get(linking.anchor)
  if (!a) return false
  if (linking.mode === 'to') return REF_TO.has(n.type ?? '')
  if (linking.mode === 'from') return REF_FROM.has(n.type ?? '')
  // a note goes under another note, but never under its own descendant
  if (a.type === 'note') return n.type === 'note' && !noteDescendantKeys(edges, a.key).has(n.key)
  // attach: a file goes to a quest / note / questline, a step to a quest
  return a.type === 'step' ? n.type === 'quest' : n.type === 'quest' || n.type === 'note' || n.type === 'questline'
}

/** Cards of one kind that belong to this card (steps of a quest, quests of a questline), as shown. */
export function childKeysOf(edges: LinkEdge[], nodes: Map<string, unknown>, key: string, type: string): string[] {
  return edges
    .filter(
      (e) =>
        e.kind === 'structure' &&
        e.from === key &&
        e.to.startsWith(`${type}:`) &&
        !e.to.includes(':t') &&
        nodes.has(e.to)
    )
    .map((e) => e.to)
}

/** The note this one hangs under, on the map. */
export function parentNoteOf(edges: LinkEdge[], key: string): string | null {
  return (
    edges.find((e) => e.kind === 'structure' && e.to === key && e.from.startsWith('note:') && e.draft !== 'remove')
      ?.from ?? null
  )
}

/** A reference from a to b that would survive saving already exists. */
export function hasRefEdge(edges: LinkEdge[], from: string, to: string): boolean {
  return edges.some((e) => e.kind === 'reference' && e.from === from && e.to === to && e.draft !== 'remove')
}

/** Links written in this card's own text, as targets still visible on the map. */
export function outgoingRefKeys(edges: LinkEdge[], nodes: Map<string, unknown>, key: string): string[] {
  return edges
    .filter((e) => e.kind === 'reference' && e.from === key && e.draft !== 'remove' && nodes.has(e.to))
    .map((e) => e.to)
}
