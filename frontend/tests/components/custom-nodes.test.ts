import { afterEach, expect, test } from 'bun:test'

import { customNodes, loadCustomNodes, nodesFor } from '@/entities/script/model/customNodes.svelte.ts'
import { createScriptEditor } from '@/pages/scripts/model/editor.svelte.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

const P = (name: string, type: string, extra: object = {}) => ({ name, type, ...extra })

// A custom node that is one Code node: a secrets input, a stdout output, a field for the fixed variable REF.
const node = {
  id: 7,
  title: 'Проверка коммита',
  version: 2,
  subjects: ['note'],
  needs_runner: true,
  inputs: [{ name: 'secrets', node: 1, port: 'secrets' }],
  outputs: [{ name: 'out', node: 1, port: 'stdout' }],
  fields: [{ name: 'ref', type: 'str', default: 'abc', node: 1, param: 'vars.REF' }],
  graph: { nodes: [{ id: 1, type: 'code', params: { text: 'echo $REF' } }], edges: [] },
}

function fakeServer() {
  const calls: { key: string; body: any }[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ key, body })
    const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200 })
    if (key === 'GET /api/script-custom-nodes')
      return json([node, { ...node, id: 8, subjects: ['step'] }, { ...node, id: 9, problem: 'broken' }])
    if (key === 'GET /api/script-nodes')
      return json([{ name: 'custom.7', title: node.title, kind: 'custom', trusted: false }])
    if (key === 'POST /api/script-nodes/ports') {
      if (body.type === 'custom.7')
        return json({ inputs: [P('secrets', 'secrets', { optional: true })], outputs: [P('out', 'str')] })
      if (body.type === 'code') {
        return json({
          inputs: [P('secrets', 'secrets', { optional: true }), P('in', 'value', { optional: true, multi: true })],
          outputs: [P('stdout', 'str')],
        })
      }
      return json({
        inputs: [P('secrets', 'secrets', { optional: true })],
        outputs: [P('ref', 'ref<note>'), P('secrets', 'secrets')],
      })
    }
    if (key === 'POST /api/scripts/validate') return json({ ok: true, problems: [] })
    if (key === 'PUT /api/script-custom-nodes/7') return json({ ...node, ...body, version: body.version + 1 })
    if (key === 'POST /api/script-custom-nodes') return json({ ...node, ...body, id: 11, version: 1 })
    return new Response(JSON.stringify({ detail: `no route ${key}` }), { status: 404 })
  }) as typeof fetch
  return calls
}

const script = (graph: any) =>
  ({
    id: 3,
    title: 'с нодой',
    description: '',
    purpose: 'note',
    version: 1,
    subjects: [],
    needs_runner: true,
    graph,
  }) as any

test('the list for pickers: only nodes that read the kind and resolve', async () => {
  fakeServer()
  await loadCustomNodes(true)
  expect(customNodes.items).toHaveLength(3)
  expect(nodesFor('note').map((n) => n.id)).toEqual([7])
  expect(nodesFor('step').map((n) => n.id)).toEqual([8])
  expect(nodesFor('template')).toEqual([])
})

test('unfolding a custom node puts its parts in its place, fields written in', async () => {
  fakeServer()
  const ed = createScriptEditor()
  await ed.open(
    script({
      nodes: [
        { id: 1, type: 'source.note' },
        { id: 2, type: 'custom.7', params: { ref: 'zzz' } },
      ],
      edges: [{ from: { node: 1, port: 'secrets' }, to: { node: 2, port: 'secrets' } }],
    })
  )
  await ed.unfold(2)
  expect(ed.graph.nodes.map((n) => [n.id, n.type])).toEqual([
    [1, 'source.note'],
    [3, 'code'],
  ])
  expect((ed.graph.nodes[1].params as any).vars).toEqual({ REF: 'zzz' })
  expect(ed.graph.edges).toEqual([{ from: { node: 1, port: 'secrets' }, to: { node: 3, port: 'secrets' } }])
  expect(ed.dirty).toBe(true)
})

test('baking over an existing node updates it by number and takes its place', async () => {
  const calls = fakeServer()
  const ed = createScriptEditor()
  await ed.open(
    script({
      nodes: [
        { id: 1, type: 'source.note' },
        { id: 2, type: 'code', params: { text: 'echo 1' } },
      ],
      edges: [{ from: { node: 1, port: 'secrets' }, to: { node: 2, port: 'secrets' } }],
    })
  )
  ed.picked = new Set(['n2'])
  const made = await ed.bake('Проверка коммита', [], 7)
  const put = calls.find((c) => c.key === 'PUT /api/script-custom-nodes/7')
  expect(put?.body.version).toBe(2)
  expect(made.version).toBe(3)
  expect(ed.graph.nodes.map((n) => n.type)).toEqual(['source.note', 'custom.7'])
  expect(calls.some((c) => c.key === 'POST /api/script-custom-nodes')).toBe(false)
})

test('a whole graph is put in with its nodes picked; right-click removal takes the nodes it is given', async () => {
  fakeServer()
  const ed = createScriptEditor()
  await ed.open(script({ nodes: [{ id: 1, type: 'source.note' }], edges: [] }))
  await ed.insert(
    {
      nodes: [
        { id: 1, type: 'code', params: { text: 'echo 1' }, x: 0, y: 0 },
        { id: 2, type: 'code', params: { text: 'echo 2' }, x: 100, y: 0 },
      ],
      edges: [{ from: { node: 1, port: 'stdout' }, to: { node: 2, port: 'in' } }],
    },
    400,
    200
  )
  expect(ed.graph.nodes.map((n) => n.id)).toEqual([1, 2, 3])
  expect(ed.graph.edges).toEqual([{ from: { node: 2, port: 'stdout' }, to: { node: 3, port: 'in' } }])
  expect([...ed.picked]).toEqual(['n2', 'n3'])
  await ed.removeNodes([2, 3])
  expect(ed.graph.nodes.map((n) => n.id)).toEqual([1])
  expect(ed.picked.size).toBe(0)
})

test('baking the whole script makes a node and leaves the script alone', async () => {
  const calls = fakeServer()
  const ed = createScriptEditor()
  const g = {
    nodes: [
      { id: 1, type: 'source.note' },
      { id: 2, type: 'code', params: { text: 'echo 1' } },
    ],
    edges: [],
  }
  await ed.open(script(g))
  const before = ed.graph
  const made = await ed.bake('Весь скрипт', [], undefined, true)
  expect(made.id).toBe(11)
  expect(calls.find((c) => c.key === 'POST /api/script-custom-nodes')?.body.graph.nodes.map((n: any) => n.id)).toEqual([
    1, 2,
  ])
  expect(ed.graph).toBe(before)
})

test('an empty script is not a problem', async () => {
  const calls = fakeServer()
  const ed = createScriptEditor()
  await ed.open(script({ nodes: [], edges: [] }))
  await ed.add('code', 0, 0)
  await ed.removeNodes([1])
  await new Promise((r) => setTimeout(r, 450))
  expect(ed.problems).toEqual([])
  expect(ed.graph.nodes).toEqual([])
  expect(calls.length).toBeGreaterThan(0)
})
