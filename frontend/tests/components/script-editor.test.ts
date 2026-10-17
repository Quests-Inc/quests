import { afterEach, expect, test } from 'bun:test'

import { createScriptEditor } from '@/pages/scripts/model/editor.svelte.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

const P = (name: string, type: string, extra: object = {}) => ({ name, type, ...extra })

// What the server says a node of a type has; Parse follows its schema, as the real one does.
function portsFor(type: string, params: any) {
  switch (type) {
    case 'source.step':
      return { inputs: [], outputs: [P('ref', 'ref<step>'), P('secrets', 'secrets'), P('progress', 'int')] }
    case 'code':
      return {
        inputs: [P('secrets', 'secrets', { optional: true }), P('in', 'value', { optional: true, multi: true })],
        outputs: [P('stdout', 'str'), P('failed', 'doc')],
      }
    case 'parse': {
      const fields = (params?.schema ?? []).map((f: any) => P(f.name, f.type, { optional: !f.required }))
      return { inputs: [P('in', 'str')], outputs: [...fields, P('bad_output', 'str')] }
    }
    case 'sink.step_progress':
      return { inputs: [P('ref', 'ref<step>'), P('progress', 'int', { optional: true, group: 'data' })], outputs: [] }
  }
  return { inputs: [], outputs: [] }
}

function fakeServer() {
  const calls: { key: string; body: any }[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ key, body })
    const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200 })
    if (key === 'POST /api/script-nodes/ports') return json(portsFor(body.type, body.params))
    if (key === 'GET /api/script-nodes')
      return json([
        { name: 'source.step', title: 'Источник: step', kind: 'source', trusted: true },
        { name: 'code', title: 'Код (раннер)', kind: 'code', trusted: false },
      ])
    if (key === 'POST /api/scripts/validate') return json({ ok: true, problems: [] })
    if (key === 'PUT /api/scripts/7')
      return json({
        id: 7,
        version: body.version + 1,
        title: body.title,
        description: '',
        subjects: ['step'],
        graph: body.graph,
      })
    return new Response(JSON.stringify({ detail: `no route ${key}` }), { status: 404 })
  }) as typeof fetch
  return calls
}

const script = (graph: any) =>
  ({ id: 7, title: 'скрипт', description: '', version: 3, subjects: ['step'], needs_runner: true, graph }) as any
const graph = {
  nodes: [
    { id: 1, type: 'source.step' },
    { id: 2, type: 'code', params: { text: 'echo 1' } },
    { id: 3, type: 'parse', params: { schema: [{ name: 'progress', type: 'int' }] } },
    { id: 4, type: 'sink.step_progress' },
  ],
  edges: [
    { from: { node: 2, port: 'stdout' }, to: { node: 3, port: 'in' } },
    { from: { node: 3, port: 'progress' }, to: { node: 4, port: 'progress' } },
    { from: { node: 1, port: 'ref' }, to: { node: 4, port: 'ref' } },
  ],
}

test('открытие: порты берутся у сервера по типу и параметрам, ноды получают места', async () => {
  const calls = fakeServer()
  const ed = createScriptEditor()
  await ed.open(script(graph))
  expect(ed.title).toBe('скрипт')
  expect(ed.version).toBe(3)
  expect(ed.graph.nodes.every((n) => n.x !== undefined && n.y !== undefined)).toBe(true)
  expect(ed.portsOf(3)?.outputs.map((p) => p.name)).toEqual(['progress', 'bad_output'])
  expect(ed.dirty).toBe(false)
  expect(calls.filter((c) => c.key === 'POST /api/script-nodes/ports')).toHaveLength(4)
  expect(ed.meta('code')).toMatchObject({ title: 'Код (раннер)', trusted: false })
})

test('правки: нода, связь, отказ по типу, отмена и повтор', async () => {
  fakeServer()
  const ed = createScriptEditor()
  await ed.open(script(graph))
  const before = ed.graph

  const id = await ed.add('code', 10, 20)
  expect(ed.graph.nodes.at(-1)).toMatchObject({ id, type: 'code', x: 10, y: 20 })
  expect(ed.selectedNode).toBe(id)
  expect(ed.dirty).toBe(true)

  // str нельзя в int-вход приёмника; сообщение остаётся в error
  expect(await ed.link({ node: id, port: 'stdout' }, { node: 4, port: 'progress' })).toBe(false)
  expect(ed.error).toContain('не подходит')
  expect(await ed.link({ node: 1, port: 'secrets' }, { node: id, port: 'secrets' })).toBe(true)
  expect(ed.error).toBe('')

  ed.undo()
  ed.undo()
  expect(ed.graph).toBe(before)
  expect(ed.canUndo).toBe(false)
  expect(ed.canRedo).toBe(true)
  ed.redo()
  expect(ed.graph.nodes).toHaveLength(5)
})

test('смена схемы разбора убирает связи с портов, которых больше нет', async () => {
  fakeServer()
  const ed = createScriptEditor()
  await ed.open(script(graph))
  expect(ed.graph.edges).toHaveLength(3)
  await ed.params(3, { schema: [{ name: 'total', type: 'int' }] })
  expect(ed.portsOf(3)?.outputs.map((p) => p.name)).toEqual(['total', 'bad_output'])
  expect(ed.graph.edges.some((e) => e.from.node === 3 && e.from.port === 'progress')).toBe(false)
  expect(ed.graph.edges).toHaveLength(2)
})

test('удаление выбранного, перетаскивание одним шагом и сохранение с версией', async () => {
  const calls = fakeServer()
  const ed = createScriptEditor()
  await ed.open(script(graph))

  ed.moved({ id: 1, x: 500, y: 400, w: 220, h: 100 })
  ed.moved({ id: 2, x: 700, y: 400, w: 220, h: 100 })
  await Promise.resolve()
  await Promise.resolve()
  expect(ed.graph.nodes.find((n) => n.id === 1)).toMatchObject({ x: 390, y: 350 })
  ed.undo() // обе ноды вернулись одним шагом
  expect(ed.graph.nodes.find((n) => n.id === 2)?.x).not.toBe(590)

  ed.select(4)
  await ed.remove()
  expect(ed.graph.nodes.map((n) => n.id)).toEqual([1, 2, 3])
  expect(ed.graph.edges.every((e) => e.to.node !== 4)).toBe(true)

  ed.title = 'новое имя'
  expect(await ed.save()).toBe(true)
  const put = calls.find((c) => c.key === 'PUT /api/scripts/7')
  expect(put?.body).toMatchObject({ version: 3, title: 'новое имя' })
  expect(ed.version).toBe(4)
  expect(ed.dirty).toBe(false)
})

test('проверка графа: проблемы по нодам', async () => {
  fakeServer()
  const ed = createScriptEditor()
  await ed.open(script(graph))
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ ok: false, problems: [{ node: 3, message: 'нет входа' }, { message: 'нет приёмника' }] }),
      { status: 200 }
    )) as typeof fetch
  await ed.checkNow()
  expect(ed.problems).toHaveLength(2)
  expect(ed.problemNodes.get(3)).toEqual(['нет входа'])
  expect(ed.problemNodes.has(0)).toBe(false)
})
