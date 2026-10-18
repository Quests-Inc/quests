import { expect, test } from 'bun:test'

import {
  bakeSelection,
  candidates,
  codeNodeDef,
  codeSpecOf,
  customIdOf,
  fieldValue,
  inlineCustom,
  replaceSelection,
  singleNodeGraph,
  slug,
} from '../../src/entities/script/model/customNode.ts'

test('names are made safe and unique', () => {
  const taken = new Set()
  expect(slug('Hash коммита', taken)).toBe('hash')
  expect(slug('hash', taken)).toBe('hash_2')
  expect(slug('123', new Set())).toBe('p123')
  expect(slug('Коммит', new Set())).toBe('p')
})

test('custom node types round-trip', () => {
  expect(customIdOf('custom.12')).toBe(12)
  expect(customIdOf('custom.x')).toBeNull()
  expect(customIdOf('code')).toBeNull()
})

const spec = {
  title: ' Свежесть ',
  description: '',
  language: 'sh',
  text: 'echo $REF',
  inputs: ['Note ID'],
  fields: [
    { name: 'ref', label: 'Коммит', type: 'str', value: 'abc' },
    { name: 'n', label: 'Число', type: 'int', value: '3' },
  ],
}

test('a code node: inputs are variables, fields are fixed variables', () => {
  const d = codeNodeDef(spec)
  expect(d.title).toBe('Свежесть')
  expect(d.inputs).toEqual([
    { name: 'secrets', node: 1, port: 'secrets' },
    { name: 'note_id', node: 1, port: 'in' },
  ])
  expect(d.outputs.map((o) => o.name)).toEqual(['stdout', 'failed'])
  expect(d.fields.map((f) => [f.name, f.param, f.default])).toEqual([
    ['ref', 'vars.REF', 'abc'],
    ['n', 'vars.N', 3],
  ])
  expect(d.graph.nodes[0].params).toEqual({ language: 'sh', text: 'echo $REF', vars: { REF: 'abc', N: '3' } })
})

test('a code node reads back into its form', () => {
  const back = codeSpecOf(codeNodeDef(spec))
  expect(back.inputs).toEqual(['note_id'])
  expect(back.fields.map((f) => [f.name, f.type, f.value])).toEqual([
    ['ref', 'str', 'abc'],
    ['n', 'int', '3'],
  ])
  expect(codeSpecOf({ ...codeNodeDef(spec), graph: { nodes: [{ id: 1, type: 'parse' }], edges: [] } })).toBeNull()
})

// source(1) → code(2) → parse(3) → sink(4); the sink also wants the source's ref.
const graph = {
  nodes: [
    { id: 1, type: 'source.note', params: { id: 5 }, x: 0, y: 0 },
    { id: 2, type: 'code', params: { text: 'echo 1', vars: { REF: 'abc' } }, x: 100, y: 0 },
    { id: 3, type: 'parse', params: { schema: [] }, x: 200, y: 0 },
    { id: 4, type: 'sink.note_error', x: 300, y: 0 },
  ],
  edges: [
    { from: { node: 1, port: 'secrets' }, to: { node: 2, port: 'secrets' } },
    { from: { node: 2, port: 'stdout' }, to: { node: 3, port: 'in' } },
    { from: { node: 3, port: 'message' }, to: { node: 4, port: 'message' } },
    { from: { node: 1, port: 'ref' }, to: { node: 4, port: 'ref' } },
  ],
}
const ports = {
  2: {
    inputs: [
      { name: 'secrets', type: 'secrets', optional: true },
      { name: 'in', type: 'value', optional: true, multi: true },
    ],
    outputs: [],
  },
  3: { inputs: [{ name: 'in', type: 'str' }], outputs: [] },
}
const portsOf = (id) => ports[id]

test('candidates are the scalar parameters, the code variables and a source id', () => {
  const c = candidates(graph, [1, 2])
  expect(c.map((x) => `${x.node}:${x.param}:${x.type}`)).toEqual(['1:id:int', '2:text:str', '2:vars.REF:str'])
  expect(candidates({ nodes: [{ id: 1, type: 'source.step' }], edges: [] }, [1]).map((x) => x.param)).toEqual(['id'])
})

test('baking the code and the parse: crossing links become ports, chosen parameters fields', () => {
  const chosen = [
    {
      candidate: { node: 2, nodeType: 'code', param: 'vars.REF', type: 'str', value: 'abc' },
      name: '',
      label: 'Коммит',
    },
  ]
  const { def, map } = bakeSelection(graph, [2, 3], portsOf, 'Разбор', chosen)
  expect(def.graph.nodes.map((n) => n.id)).toEqual([2, 3])
  expect(def.graph.edges).toEqual([{ from: { node: 2, port: 'stdout' }, to: { node: 3, port: 'in' } }])
  expect(def.inputs).toEqual([{ name: 'secrets', node: 2, port: 'secrets' }])
  expect(def.outputs).toEqual([{ name: 'message', node: 3, port: 'message' }])
  expect(def.fields).toEqual([
    { name: 'ref', label: 'Коммит', type: 'str', default: 'abc', node: 2, param: 'vars.REF' },
  ])
  expect(map.open).toEqual([])

  const out = replaceSelection(graph, [2, 3], map, 9, 5)
  expect(out.nodes.map((n) => [n.id, n.type])).toEqual([
    [1, 'source.note'],
    [4, 'sink.note_error'],
    [5, 'custom.9'],
  ])
  expect(out.edges).toEqual([
    { from: { node: 1, port: 'ref' }, to: { node: 4, port: 'ref' } },
    { from: { node: 1, port: 'secrets' }, to: { node: 5, port: 'secrets' } },
    { from: { node: 5, port: 'message' }, to: { node: 4, port: 'message' } },
  ])
})

test('an input nothing wires and the node needs is exposed as an open input', () => {
  const { def } = bakeSelection(graph, [3], portsOf, 'Разбор', [])
  // parse.in is wired from outside (node 2), so it is an input by link, not an open one
  expect(def.inputs).toEqual([{ name: 'in', node: 3, port: 'in' }])
  const lone = { nodes: [{ id: 3, type: 'parse' }], edges: [] }
  const b = bakeSelection(lone, [3], portsOf, 'Разбор', [])
  expect(b.def.inputs).toEqual([{ name: 'in', node: 3, port: 'in' }])
  expect(b.map.open).toEqual(['in'])
})

test("field values: the node's own, else the baked default", () => {
  const f = { name: 'ref', type: 'str', default: 'abc', node: 1, param: 'vars.REF' }
  expect(fieldValue(f, { ref: 'x' })).toBe('x')
  expect(fieldValue(f, undefined)).toBe('abc')
  expect(fieldValue({ name: 'b', type: 'bool', node: 1, param: 'x' }, {})).toBe(false)
})

test('a one-node script carries the entity number when the node has a field for it', () => {
  const node = { id: 4, fields: [{ name: 'entity_id', type: 'int', node: 1, param: 'id' }] }
  expect(singleNodeGraph(node, 9)).toEqual({
    nodes: [{ id: 1, type: 'custom.4', params: { entity_id: 9 } }],
    edges: [],
  })
  expect(singleNodeGraph({ id: 4, fields: [] }, 9).nodes[0].params).toBeUndefined()
  expect(singleNodeGraph(node).nodes[0].params).toBeUndefined()
})

test('a custom node is unfolded into its parts, fields written, links rewired', () => {
  const def = codeNodeDef(spec)
  const outer = {
    nodes: [
      { id: 1, type: 'source.note', x: 0, y: 0 },
      { id: 2, type: 'custom.7', params: { ref: 'zzz' }, x: 300, y: 100 },
      { id: 3, type: 'sink.done', x: 600, y: 100 },
    ],
    edges: [
      { from: { node: 1, port: 'secrets' }, to: { node: 2, port: 'secrets' } },
      { from: { node: 1, port: 'title' }, to: { node: 2, port: 'note_id' } },
      { from: { node: 2, port: 'stdout' }, to: { node: 3, port: 'trigger' } },
    ],
  }
  const out = inlineCustom(outer, 2, def)
  expect(out.nodes.map((n) => [n.id, n.type])).toEqual([
    [1, 'source.note'],
    [3, 'sink.done'],
    [4, 'code'],
  ])
  expect(out.nodes[2].params.vars).toEqual({ REF: 'zzz', N: '3' })
  expect(out.nodes[2].x).toBe(300)
  expect(out.edges).toEqual([
    { from: { node: 1, port: 'secrets' }, to: { node: 4, port: 'secrets' } },
    { from: { node: 1, port: 'title' }, to: { node: 4, port: 'in' }, alias: 'NOTE_ID' },
    { from: { node: 4, port: 'stdout' }, to: { node: 3, port: 'trigger' } },
  ])
})
