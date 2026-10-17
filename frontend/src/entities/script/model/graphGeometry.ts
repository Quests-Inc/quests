import type { Graph } from '@/shared/api/scriptgraph.gen.ts'

import type { NodePorts, PortsOf } from './graphEdit.ts'

/** Sizes of a node box, in stage pixels: edges are drawn from these, not measured from the page. */
export const NODE_W = 220
export const HEAD_H = 30
export const ROW_H = 24
export const PAD_B = 8
const GAP_X = 90
const GAP_Y = 36

export function nodeHeight(p: NodePorts | undefined): number {
  const rows = Math.max(p?.inputs.length ?? 0, p?.outputs.length ?? 0, 1)
  return HEAD_H + rows * ROW_H + PAD_B
}

/**
 * A node as the canvas draws and drags it. The map's gestures move anything shaped like this (a key and a box given by its
 * centre and size), so the script editor shares them; the rest is what the box shows.
 */
export interface ScriptNodeView {
  key: string
  id: number
  type: string
  title: string
  kind: string
  color: string
  trusted: boolean
  x: number
  y: number
  w: number
  h: number
  /** the fields the map's node type requires, unused here */
  links: number
  depth: number
}

export const nodeKey = (id: number) => `n${id}`

/** The views of every node of a graph (positioned ones; run `layout` first). */
export function viewNodes(
  g: Graph,
  portsOf: PortsOf,
  meta: (type: string) => { title: string; kind: string; color: string; trusted: boolean }
): ScriptNodeView[] {
  return g.nodes.map((n) => {
    const h = nodeHeight(portsOf(n.id))
    const m = meta(n.type)
    return {
      key: nodeKey(n.id),
      id: n.id,
      type: n.type,
      title: m.title,
      kind: m.kind,
      color: m.color,
      trusted: m.trusted,
      x: (n.x ?? 0) + NODE_W / 2,
      y: (n.y ?? 0) + h / 2,
      w: NODE_W,
      h,
      links: 0,
      depth: 0,
    }
  })
}

/** Write the positions the canvas moved nodes to back into the graph. Returns the same graph when nothing moved. */
export function commitPositions(g: Graph, views: Pick<ScriptNodeView, 'id' | 'x' | 'y' | 'w' | 'h'>[]): Graph {
  const at = new Map(views.map((v) => [v.id, v]))
  let changed = false
  const nodes = g.nodes.map((n) => {
    const v = at.get(n.id)
    if (!v) return n
    const x = Math.round(v.x - v.w / 2)
    const y = Math.round(v.y - v.h / 2)
    if (x === n.x && y === n.y) return n
    changed = true
    return { ...n, x, y }
  })
  return changed ? { ...g, nodes } : g
}

/** Where a port's anchor is, in the world, for a node drawn at `v`. */
export function anchorOf(
  v: Pick<ScriptNodeView, 'x' | 'y' | 'w' | 'h'>,
  p: NodePorts | undefined,
  side: 'in' | 'out',
  port: string
): { x: number; y: number } {
  const list = (side === 'in' ? p?.inputs : p?.outputs) ?? []
  const i = Math.max(
    0,
    list.findIndex((q) => q.name === port)
  )
  return { x: v.x + (side === 'out' ? v.w / 2 : -v.w / 2), y: v.y - v.h / 2 + HEAD_H + i * ROW_H + ROW_H / 2 }
}

/** A smooth wire between two anchors. */
export function wirePath(a: { x: number; y: number }, b: { x: number; y: number }): string {
  const dx = Math.max(40, Math.abs(b.x - a.x) / 2)
  return `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`
}

/** The depth of every node: the longest chain of links that ends in it (sources are 0). */
export function depths(g: Graph): Map<number, number> {
  const d = new Map<number, number>()
  const incoming = new Map<number, number[]>()
  for (const e of g.edges) incoming.set(e.to.node, [...(incoming.get(e.to.node) ?? []), e.from.node])
  const visit = (id: number, seen: Set<number>): number => {
    if (d.has(id)) return d.get(id) as number
    if (seen.has(id)) return 0
    seen.add(id)
    const v = Math.max(-1, ...(incoming.get(id) ?? []).map((p) => visit(p, seen))) + 1
    d.set(id, v)
    return v
  }
  for (const n of g.nodes) visit(n.id, new Set())
  return d
}

/** Give a position to every node that has none: columns by depth, rows in order. Positioned nodes stay where they are. */
export function layout(g: Graph, portsOf: PortsOf): Graph {
  if (g.nodes.every((n) => n.x !== undefined && n.y !== undefined)) return g
  const d = depths(g)
  const rows = new Map<number, number>()
  const nodes = g.nodes.map((n) => {
    if (n.x !== undefined && n.y !== undefined) return n
    const col = d.get(n.id) ?? 0
    const y = rows.get(col) ?? 0
    rows.set(col, y + nodeHeight(portsOf(n.id)) + GAP_Y)
    return { ...n, x: 40 + col * (NODE_W + GAP_X), y: 40 + y }
  })
  return { ...g, nodes }
}

/** The stage size that holds every node with room to grow. */
export function stageSize(g: Graph, portsOf: PortsOf): { w: number; h: number } {
  let w = 900
  let h = 560
  for (const n of g.nodes) {
    w = Math.max(w, (n.x ?? 0) + NODE_W + 240)
    h = Math.max(h, (n.y ?? 0) + nodeHeight(portsOf(n.id)) + 200)
  }
  return { w, h }
}
