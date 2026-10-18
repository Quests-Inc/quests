import type { Edge, End, Graph, Node, Port } from '@/shared/api/scriptgraph.gen.ts'

import { assignable } from './portTypes.ts'

/** The ports of a node as the server resolved them from its type and parameters. */
export interface NodePorts {
  inputs: Port[]
  outputs: Port[]
  /** why the server could not resolve the ports (wrong parameters); the node is then drawn empty */
  error?: string
}
export type PortsOf = (nodeId: number) => NodePorts | undefined

/** A graph is edited as a value: every function here returns a new one and leaves its argument alone. */

export function nextNodeId(g: Graph): number {
  return g.nodes.reduce((m, n) => Math.max(m, n.id), 0) + 1
}

export function addNode(g: Graph, type: string, params: unknown, x: number, y: number): { graph: Graph; id: number } {
  const id = nextNodeId(g)
  const node: Node = { id, type, x: Math.round(x), y: Math.round(y) }
  if (params !== undefined) node.params = params
  return { graph: { ...g, nodes: [...g.nodes, node] }, id }
}

export function removeNode(g: Graph, id: number): Graph {
  return {
    nodes: g.nodes.filter((n) => n.id !== id),
    edges: g.edges.filter((e) => e.from.node !== id && e.to.node !== id),
  }
}

export function moveNode(g: Graph, id: number, x: number, y: number): Graph {
  return { ...g, nodes: g.nodes.map((n) => (n.id === id ? { ...n, x: Math.round(x), y: Math.round(y) } : n)) }
}

export function setParams(g: Graph, id: number, params: unknown): Graph {
  return { ...g, nodes: g.nodes.map((n) => (n.id === id ? { ...n, params } : n)) }
}

const sameEnd = (a: End, b: End) => a.node === b.node && a.port === b.port

/** Whether a link from `from` to `to` would close a cycle (`to` already leads back to `from`). */
export function wouldCycle(g: Graph, from: number, to: number): boolean {
  if (from === to) return true
  const seen = new Set<number>()
  const stack = [to]
  while (stack.length) {
    const n = stack.pop() as number
    if (n === from) return true
    if (seen.has(n)) continue
    seen.add(n)
    for (const e of g.edges) if (e.from.node === n) stack.push(e.to.node)
  }
  return false
}

export type Connected = { graph: Graph; error?: undefined } | { graph: Graph; error: string }

/**
 * Link an output to an input. An input that takes one link gets this one in place of the old; refused with a reason when a
 * port is missing, the types do not fit, or the link would close a cycle.
 */
export function connect(g: Graph, from: End, to: End, portsOf: PortsOf): Connected {
  const out = portsOf(from.node)?.outputs.find((p) => p.name === from.port)
  const inp = portsOf(to.node)?.inputs.find((p) => p.name === to.port)
  if (!out || !inp) return { graph: g, error: 'Такого порта нет' }
  if (!assignable(out.type, inp.type)) return { graph: g, error: `Тип «${out.type}» не подходит входу «${inp.type}»` }
  if (wouldCycle(g, from.node, to.node)) return { graph: g, error: 'Связь замкнула бы цикл' }
  if (g.edges.some((e) => sameEnd(e.from, from) && sameEnd(e.to, to)))
    return { graph: g, error: 'Такая связь уже есть' }
  const kept = inp.multi ? g.edges : g.edges.filter((e) => !sameEnd(e.to, to))
  return { graph: { ...g, edges: [...kept, { from, to }] } }
}

export function disconnect(g: Graph, index: number): Graph {
  return { ...g, edges: g.edges.filter((_, i) => i !== index) }
}

export function setEdge(g: Graph, index: number, patch: Partial<Pick<Edge, 'alias' | 'optional'>>): Graph {
  return {
    ...g,
    edges: g.edges.map((e, i) => {
      if (i !== index) return e
      const next = { ...e, ...patch }
      if (!next.alias) delete next.alias
      if (!next.optional) delete next.optional
      return next
    }),
  }
}

/** Drop the links whose ports no longer exist or no longer fit: what is left after a node's parameters changed its ports. */
export function pruneEdges(g: Graph, portsOf: PortsOf): Graph {
  const edges = g.edges.filter((e) => {
    const out = portsOf(e.from.node)?.outputs.find((p) => p.name === e.from.port)
    const inp = portsOf(e.to.node)?.inputs.find((p) => p.name === e.to.port)
    return Boolean(out && inp && assignable(out.type, inp.type))
  })
  return edges.length === g.edges.length ? g : { ...g, edges }
}

/** A key for the ports a node resolves to: the type and its parameters, so equal nodes share one lookup. */
export function portsKey(n: Pick<Node, 'type' | 'params'>): string {
  return `${n.type}|${n.params === undefined ? '' : JSON.stringify(n.params)}`
}

/**
 * Put another graph (a sample, another script) into this one: its nodes are numbered after ours, its links come with them, and
 * its middle lands on (x, y). The new nodes' numbers are returned so the editor can pick them.
 */
export function appendGraph(g: Graph, extra: Graph, x: number, y: number): { graph: Graph; ids: number[] } {
  if (!extra.nodes.length) return { graph: g, ids: [] }
  let next = nextNodeId(g) - 1
  const renum = new Map<number, number>()
  for (const n of extra.nodes) renum.set(n.id, ++next)
  const xs = extra.nodes.map((n) => n.x ?? 0)
  const ys = extra.nodes.map((n) => n.y ?? 0)
  const mid = (a: number[]) => (Math.min(...a) + Math.max(...a)) / 2
  const dx = x - mid(xs)
  const dy = y - mid(ys)
  const nodes: Node[] = extra.nodes.map((n) => ({
    ...n,
    id: renum.get(n.id) as number,
    x: Math.round((n.x ?? 0) + dx),
    y: Math.round((n.y ?? 0) + dy),
  }))
  const edges: Edge[] = extra.edges.map((e) => ({
    ...e,
    from: { ...e.from, node: renum.get(e.from.node) as number },
    to: { ...e.to, node: renum.get(e.to.node) as number },
  }))
  return { graph: { nodes: [...g.nodes, ...nodes], edges: [...g.edges, ...edges] }, ids: nodes.map((n) => n.id) }
}
