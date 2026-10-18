import type { Custom, CustomField, CustomPort, Edge, Graph, Node } from '@/shared/api/scriptgraph.gen.ts'

import type { PortsOf } from './graphEdit.ts'

/**
 * Custom nodes (quest=458): a definition is a graph fragment, which inner ports are the node's inputs and outputs, and which
 * inner parameters are editable fields. Here the definitions are built from what the user typed or selected; the server
 * checks them and expands the node into plain nodes before a run.
 */

export type CustomDef = Pick<Custom, 'title' | 'inputs' | 'outputs' | 'fields' | 'graph'> & {
  description?: string
  version?: number
}

export const CUSTOM_PREFIX = 'custom.'
export const customType = (id: number) => `${CUSTOM_PREFIX}${id}`
export const customIdOf = (type: string): number | null => {
  if (!type.startsWith(CUSTOM_PREFIX)) return null
  const id = Number(type.slice(CUSTOM_PREFIX.length))
  return Number.isInteger(id) && id > 0 ? id : null
}

const NAME_RE = /^[a-z][a-z0-9_]{0,31}$/

/** A name the server accepts for an input, output or field, made from any text (`Hash коммита` → `hash`). */
export function slug(text: string, taken: Set<string> = new Set()): string {
  let base = text
    .toLowerCase()
    .replace(/[^a-z0-9_]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 28)
  if (!NAME_RE.test(base)) base = `p${base}`.replace(/_+$/, '')
  if (!NAME_RE.test(base)) base = 'p'
  let name = base
  for (let i = 2; taken.has(name); i++) name = `${base}_${i}`
  taken.add(name)
  return name
}

// ---- a node written as code -------------------------------------------------------------------------------------------

export type CodeField = { name: string; label: string; type: 'str' | 'int' | 'bool'; value: string }
export type CodeSpec = {
  title: string
  description: string
  language: string
  text: string
  /** names of the values the node takes; each arrives as the variable of the same name in capitals */
  inputs: string[]
  fields: CodeField[]
}

const upper = (s: string) => s.toUpperCase()

function fieldDefault(f: { type: string; value: string }): string | number | boolean {
  if (f.type === 'int') return Math.trunc(Number(f.value) || 0)
  if (f.type === 'bool') return f.value === 'true'
  return f.value
}

/** The definition of a node that is one Code node: its inputs are variables, its fields fixed variables. */
export function codeNodeDef(spec: CodeSpec): CustomDef {
  const vars: Record<string, string> = {}
  const taken = new Set<string>(['secrets'])
  const inputs: CustomPort[] = [{ name: 'secrets', node: 1, port: 'secrets' }]
  for (const raw of spec.inputs.map((s) => s.trim()).filter(Boolean)) {
    inputs.push({ name: slug(raw, taken), node: 1, port: 'in' })
  }
  const fields: CustomField[] = []
  const fieldNames = new Set<string>()
  for (const f of spec.fields) {
    const name = slug(f.name || f.label, fieldNames)
    vars[upper(name)] = String(fieldDefault(f))
    fields.push({
      name,
      label: f.label || name,
      type: f.type,
      default: fieldDefault(f),
      node: 1,
      param: `vars.${upper(name)}`,
    })
  }
  const params: Record<string, unknown> = { language: spec.language, text: spec.text }
  if (Object.keys(vars).length) params.vars = vars
  return {
    title: spec.title.trim(),
    description: spec.description.trim() || undefined,
    inputs,
    outputs: [
      { name: 'stdout', node: 1, port: 'stdout' },
      { name: 'failed', node: 1, port: 'failed' },
    ],
    fields,
    graph: { nodes: [{ id: 1, type: 'code', params }], edges: [] },
  }
}

/** Reads a definition back into the code form, when it has that shape (one Code node); otherwise null. */
export function codeSpecOf(def: CustomDef): CodeSpec | null {
  const [n] = def.graph.nodes
  if (def.graph.nodes.length !== 1 || n.type !== 'code' || def.graph.edges.length) return null
  const p = (n.params ?? {}) as { language?: string; text?: string }
  const fields: CodeField[] = def.fields.map((f) => ({
    name: f.name,
    label: f.label ?? f.name,
    type: f.type as CodeField['type'],
    value: String(f.default ?? ''),
  }))
  return {
    title: def.title,
    description: def.description ?? '',
    language: p.language ?? '',
    text: p.text ?? '',
    inputs: def.inputs.filter((i) => i.name !== 'secrets').map((i) => i.name),
    fields,
  }
}

// ---- baking selected nodes ----------------------------------------------------------------------------------------------

/** A parameter of a selected node the user may turn into a field of the baked node. */
export type Candidate = {
  node: number
  nodeType: string
  param: string
  type: 'str' | 'int' | 'bool'
  value: string
}

/** The scalar parameters of the selected nodes (and their Code variables), plus a source's entity number. */
export function candidates(g: Graph, ids: number[]): Candidate[] {
  const out: Candidate[] = []
  for (const n of g.nodes.filter((x) => ids.includes(x.id))) {
    const p = (n.params ?? {}) as Record<string, unknown>
    for (const [k, v] of Object.entries(p)) {
      if (k === 'vars' && v && typeof v === 'object') {
        for (const [name, val] of Object.entries(v as Record<string, unknown>)) {
          out.push({ node: n.id, nodeType: n.type, param: `vars.${name}`, type: 'str', value: String(val) })
        }
      } else if (typeof v === 'string') out.push({ node: n.id, nodeType: n.type, param: k, type: 'str', value: v })
      else if (typeof v === 'number' && Number.isInteger(v)) {
        out.push({ node: n.id, nodeType: n.type, param: k, type: 'int', value: String(v) })
      } else if (typeof v === 'boolean')
        out.push({ node: n.id, nodeType: n.type, param: k, type: 'bool', value: String(v) })
    }
    if (n.type.startsWith('source.') && n.type !== 'source.workspace' && !('id' in p)) {
      out.push({ node: n.id, nodeType: n.type, param: 'id', type: 'int', value: '' })
    }
  }
  return out
}

export type ChosenField = { candidate: Candidate; name: string; label: string }

/** How the baked node's ports map to the ports of the nodes it replaces; used to rewire the outer graph. */
export type BakeMap = {
  inputs: { port: string; outer: Edge }[]
  /** inputs that were open inside (nothing wired them): they appear on the new node unwired */
  open: string[]
  outputs: { port: string; outer: Edge }[]
}

export type Baked = { def: CustomDef; map: BakeMap }

/**
 * The definition made of the selected nodes. Links crossing the selection become inputs and outputs; inputs a selected node
 * needs and nothing wires become inputs too (the user wires them on the new node); the chosen parameters become fields.
 */
export function bakeSelection(g: Graph, ids: number[], portsOf: PortsOf, title: string, chosen: ChosenField[]): Baked {
  const inSel = new Set(ids)
  const nodes = g.nodes.filter((n) => inSel.has(n.id))
  const inner: Edge[] = g.edges.filter((e) => inSel.has(e.from.node) && inSel.has(e.to.node))
  const incoming = g.edges.filter((e) => !inSel.has(e.from.node) && inSel.has(e.to.node))
  const outgoing = g.edges.filter((e) => inSel.has(e.from.node) && !inSel.has(e.to.node))

  const taken = new Set<string>()
  const inputs: CustomPort[] = []
  const map: BakeMap = { inputs: [], open: [], outputs: [] }

  // One input per link from outside. A link into a Code node's `in` keeps its alias as the input's name.
  for (const e of incoming) {
    const code = nodes.find((n) => n.id === e.to.node)?.type === 'code' && e.to.port === 'in'
    const name = slug(code && e.alias ? e.alias : code ? `${e.from.port}_${e.from.node}` : e.to.port, taken)
    inputs.push({ name, node: e.to.node, port: e.to.port })
    map.inputs.push({ port: name, outer: e })
  }
  // Inputs nothing wires, inside or outside, that the node cannot run without.
  const wired = new Set(g.edges.map((e) => `${e.to.node}.${e.to.port}`))
  for (const n of nodes) {
    for (const p of portsOf(n.id)?.inputs ?? []) {
      if (p.optional || p.multi || wired.has(`${n.id}.${p.name}`)) continue
      const name = slug(p.name, taken)
      inputs.push({ name, node: n.id, port: p.name })
      map.open.push(name)
    }
  }
  const outputs: CustomPort[] = []
  const seen = new Map<string, string>()
  for (const e of outgoing) {
    const key = `${e.from.node}.${e.from.port}`
    let name = seen.get(key)
    if (!name) {
      name = slug(e.from.port, taken)
      seen.set(key, name)
      outputs.push({ name, node: e.from.node, port: e.from.port })
    }
    map.outputs.push({ port: name, outer: e })
  }

  const fieldNames = new Set<string>()
  const fields: CustomField[] = chosen.map((c) => {
    const f: CustomField = {
      name: slug(c.name || c.candidate.param.replace(/^vars\./, ''), fieldNames),
      label: c.label || c.candidate.param,
      type: c.candidate.type,
      node: c.candidate.node,
      param: c.candidate.param,
    }
    if (c.candidate.value !== '') f.default = fieldDefault(c.candidate)
    return f
  })

  const def: CustomDef = {
    title: title.trim(),
    inputs,
    outputs,
    fields,
    graph: {
      nodes: nodes.map(({ id, type, params }): Node => ({ id, type, ...(params === undefined ? {} : { params }) })),
      edges: inner,
    },
  }
  return { def, map }
}

/**
 * The outer graph after the selection is replaced by the custom node `customId`: the node sits where the selection was,
 * the links that crossed its border go to its ports.
 */
export function replaceSelection(g: Graph, ids: number[], map: BakeMap, customId: number, nodeId: number): Graph {
  const inSel = new Set(ids)
  const sel = g.nodes.filter((n) => inSel.has(n.id))
  const x = Math.round(sel.reduce((s, n) => s + (n.x ?? 0), 0) / Math.max(sel.length, 1))
  const y = Math.round(sel.reduce((s, n) => s + (n.y ?? 0), 0) / Math.max(sel.length, 1))
  const nodes: Node[] = [...g.nodes.filter((n) => !inSel.has(n.id)), { id: nodeId, type: customType(customId), x, y }]
  const edges: Edge[] = g.edges.filter((e) => !inSel.has(e.from.node) && !inSel.has(e.to.node))
  for (const m of map.inputs) {
    const { alias: _a, optional: _o, ...rest } = m.outer
    edges.push({ ...rest, to: { node: nodeId, port: m.port } })
  }
  for (const m of map.outputs) edges.push({ from: { node: nodeId, port: m.port }, to: m.outer.to })
  return { nodes, edges }
}

/** The values of a custom node's fields as the inspector shows them: the node's own, else the baked default. */
export function fieldValue(f: CustomField, params: unknown): string | number | boolean {
  const v = (params as Record<string, unknown> | undefined)?.[f.name]
  if (v !== undefined) return v as string | number | boolean
  if (f.default !== undefined) return f.default as string | number | boolean
  return f.type === 'bool' ? false : ''
}
