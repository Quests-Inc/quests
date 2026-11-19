import { expect, test } from 'bun:test'

import { createGraphQuery } from '@/pages/graph/model/query.svelte.ts'

function make(
  over: Record<string, unknown> = {},
  root: { type: string | null; id: number | null } = { type: 'quest', id: 5 }
) {
  const calls: string[] = []
  let active = true
  let rootType: string | null = root.type
  let rootId: number | null = root.id
  let nonce = 1
  const roots: { type: string; id: number }[] = []
  const written: { type: string; id: number }[][] = []
  let expand = ['quest:5>note']
  const pending: { resolve: (g: unknown) => void; reject: (e: unknown) => void }[] = []
  const q = createGraphQuery({
    searchResults: () => ({
      parsed: { text: 'аль', terms: [], unknown: [], active: false },
      items: [{ type: 'quest', id: 5, title: 'Альфа' }],
    }),
    getActive: () => active,
    getRootType: () => rootType,
    getRootId: () => rootId,
    getNonce: () => nonce,
    getExpandKeys: () => expand,
    getReloadTick: () => 0,
    getDraftRoots: () => roots,
    getHiddenSize: () => 0,
    getQuests: () => [{ id: 5, title: 'Альфа', status: 'active' }],
    getNotes: () => [],
    getQuestlines: () => [],
    onCenter: (type, id) => calls.push(`center:${type}:${id}`),
    onRetarget: () => calls.push('retarget'),
    prepareFresh: () => calls.push('fresh'),
    forgetPositions: () => calls.push('forget'),
    resetFolds: () => calls.push('reset'),
    setExpand: (keys) => {
      expand = keys
      calls.push('expand')
    },
    saveHidden: () => calls.push('hidden'),
    cancelLink: () => calls.push('cancel'),
    clearEditing: () => calls.push('edit'),
    clearSelection: () => calls.push('selection'),
    loadDraft: async () => {
      calls.push('draft')
    },
    fetchGraph: (type, id, depth, extra, open) => {
      calls.push(
        `fetch:${type}:${id}:${depth}:${extra.map((r) => r.type + r.id).join(',')}:${open.map((r) => r.only ?? '').join(',')}`
      )
      return new Promise((resolve, reject) => pending.push({ resolve, reject }))
    },
    writeRoots: async (next) => {
      written.push(next.map((r) => ({ ...r })))
      roots.splice(0, roots.length, ...next)
    },
    ...over,
  })
  return {
    q,
    calls,
    written,
    pending,
    setActive: (v: boolean) => {
      active = v
    },
    setRoot: (type: string | null, id: number | null, n = nonce) => {
      rootType = type
      rootId = id
      nonce = n
    },
    roots,
  }
}

const graph = {
  root: 'quest:5',
  roots: ['quest:5'],
  depth: 2,
  truncated: false,
  nodes: [],
  edges: [],
}

test('центр снаружи применяется один раз, пока не сменится ключ', () => {
  const { q, calls, setRoot } = make()
  q.syncRoot()
  q.syncRoot()
  expect(q.center).toEqual({ type: 'quest', id: 5 })
  expect(calls.filter((c) => c === 'retarget')).toEqual(['retarget'])
  setRoot('note', 8, 2)
  q.syncRoot()
  expect(q.center).toEqual({ type: 'note', id: 8 })
  expect(calls.filter((c) => c === 'retarget')).toHaveLength(2)
})

test('скрытая вкладка не запрашивает граф, пустой центр даёт пустую карту', () => {
  const { q, calls, setActive } = make()
  setActive(false)
  q.load()
  expect(calls.some((c) => c.startsWith('fetch'))).toBe(false)
  q.syncRoot()
  expect(q.center).toEqual({ type: 'quest', id: 5 })
  setActive(true)
  q.center
  const empty = make({}, { type: null, id: null })
  empty.q.load()
  expect(empty.q.data?.root).toBe('')
  expect(empty.q.loading).toBe(false)
})

test('запрос передаёт глубину, чужие центры и раскрытие; устаревший ответ отбрасывается', async () => {
  const { q, calls, pending, roots } = make()
  roots.push({ type: 'note', id: 3 }, { type: 'quest', id: 5 })
  q.load()
  q.load()
  expect(calls.at(-1)).toBe('fetch:quest:5:2:note3:note')
  pending[0].resolve({ ...graph, root: 'stale' })
  pending[1].resolve(graph)
  for (let i = 0; i < 6; i++) await Promise.resolve()
  expect(q.data?.root).toBe('quest:5')
  expect(q.loading).toBe(false)
})

test('повтор того же ответа не подменяет объект графа', async () => {
  const { q, pending } = make()
  q.load()
  pending[0].resolve(graph)
  await Promise.resolve()
  const first = q.data
  q.load()
  pending[1].resolve({ ...graph })
  await Promise.resolve()
  expect(q.data).toBe(first)
})

test('выбор корня и очистка карты', async () => {
  const { q, calls, written, roots } = make()
  q.rootQuery = 'аль'
  q.pickOpen = true
  expect(q.found.items.map((m) => m.title)).toEqual(['Альфа'])
  await q.chooseRoot({ type: 'note', id: 2 })
  expect(q.rootQuery).toBe('')
  expect(q.pickOpen).toBe(false)
  expect(q.center).toEqual({ type: 'note', id: 2 })
  expect(roots).toEqual([{ type: 'quest', id: 5 }])
  expect(written[0]).toEqual([{ type: 'quest', id: 5 }])
  expect(calls).toContain('fresh')
  await q.clearMap()
  expect(q.center).toEqual({ type: null, id: null })
  expect(written.at(-1)).toEqual([])
  expect(calls).toContain('center:null:null')
})

test('сделать центром оставляет прежнее дерево на карте', async () => {
  const { q, roots, written } = make()
  roots.push({ type: 'note', id: 79 })
  await q.recenter({ type: 'note', id: 79 })
  expect(q.center).toEqual({ type: 'note', id: 79 })
  expect(roots).toEqual([{ type: 'quest', id: 5 }])
  expect(written[0]).toEqual([{ type: 'quest', id: 5 }])
})

test('первый добавленный становится центром, повтор — нет', async () => {
  const box = make({}, { type: null, id: null })
  await box.q.addToMap('quest', 9)
  expect(box.q.center).toEqual({ type: 'quest', id: 9 })
  expect(box.calls).toContain('forget')
  await box.q.addToMap('quest', 9)
  expect(box.written).toEqual([])
  box.roots.push({ type: 'note', id: 4 })
  await box.q.addToMap('note', 4)
  expect(box.written).toEqual([])
  await box.q.addToMap('note', 8)
  expect(box.written[0]).toEqual([
    { type: 'note', id: 4 },
    { type: 'note', id: 8 },
  ])
})

test('у каждого дерева своя глубина; общие карточки и рёбра объединяются', async () => {
  const { q, roots, calls, pending } = make({
    getTreeSettings: (key: string) => ({ depth: key === 'quest:5' ? 1 : 3, layout: 'tree' }),
  })
  roots.push({ type: 'note', id: 8 })
  q.depth = 2
  q.load()
  expect(calls.filter((c) => c.startsWith('fetch'))).toEqual(['fetch:quest:5:1::note', 'fetch:note:8:3::note'])
  const shared = { key: 'note:9', type: 'note', id: 9, depth: 1, title: 'Общая' }
  pending[0].resolve({ ...graph, nodes: [shared] })
  pending[1].resolve({ ...graph, root: 'note:8', nodes: [shared], depth: 3 })
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(q.data?.nodes).toHaveLength(1)
  expect(q.memberships).toEqual({ 'quest:5': ['note:9'], 'note:8': ['note:9'] })
  const count = calls.length
  q.depth = 1
  expect(calls).toHaveLength(count)
})

test('catalog refresh does not create a new temporary layout; explicit navigation does', async () => {
  let visit = 1
  const { q, setRoot, calls } = make({ getRootNonce: () => visit })
  await q.syncRoot()
  setRoot('quest', 5, 22)
  await q.syncRoot()
  expect(calls.filter((c) => c === 'retarget')).toHaveLength(1)
  visit++
  await q.syncRoot()
  expect(calls.filter((c) => c === 'retarget')).toHaveLength(2)
})

test('adding a root prepares its visible position before updating roots', async () => {
  const order: string[] = []
  const { q } = make({
    prepareAddition: (type: string, id: number) => {
      order.push(`place:${type}:${id}`)
    },
    writeRoots: async () => {
      order.push('write')
    },
  })
  await q.addToMap('note', 9)
  expect(order).toEqual(['place:note:9', 'write'])
})

test('a slow external retarget enters loading before awaiting draft state', async () => {
  let release!: () => void
  const { q } = make({
    onRetarget: () =>
      new Promise<void>((resolve) => {
        release = resolve
      }),
  })
  const work = q.syncRoot()
  expect(q.loading).toBe(true)
  release()
  await work
  expect(q.loading).toBe(false)
})

test('restoring a board pauses queries; returning to it uses cached data while revalidating', async () => {
  const { q, pending, calls } = make()
  q.beginScene()
  q.load()
  expect(calls.filter((c) => c.startsWith('fetch'))).toHaveLength(0)
  q.endScene()
  q.load()
  pending[0].resolve(graph)
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(q.loading).toBe(false)
  q.beginScene()
  q.endScene()
  q.load()
  expect(q.data?.root).toBe('quest:5')
  expect(q.loading).toBe(false)
  expect(pending).toHaveLength(2)
  pending[1].resolve({ ...graph, truncated: true })
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(q.data?.truncated).toBe(true)
})

test('independent tree settings use a single batch request', async () => {
  const batches: unknown[] = []
  const { q, roots, calls } = make({
    getTreeSettings: (key: string) => ({ depth: key === 'quest:5' ? 1 : 3, layout: 'free' }),
    fetchGraphs: async (specs: unknown, open: unknown) => {
      batches.push([specs, open])
      return [graph, { ...graph, root: 'note:8' }]
    },
  })
  roots.push({ type: 'note', id: 8 })
  q.load()
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(batches).toHaveLength(1)
  expect(batches[0]).toEqual([
    [
      { type: 'quest', id: 5, depth: 1 },
      { type: 'note', id: 8, depth: 3 },
    ],
    [{ type: 'quest', id: 5, only: 'note' }],
  ])
  expect(calls.filter((c) => c.startsWith('fetch'))).toHaveLength(0)
})

test('dirty and reconnect revisions revalidate cached cards without blanking the board', async () => {
  const { q, pending, setRoot } = make()
  q.load()
  pending[0].resolve(graph)
  await new Promise((resolve) => setTimeout(resolve, 0))
  setRoot('quest', 5, 2)
  q.load()
  expect(q.loading).toBe(false)
  expect(q.data?.root).toBe('quest:5')
  expect(pending).toHaveLength(2)
  pending[1].resolve({ ...graph, truncated: true })
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(q.data?.truncated).toBe(true)
})

test('restoring an empty centre does not clear saved roots or reset board preferences', () => {
  const { q, roots, calls, written } = make()
  roots.push({ type: 'note', id: 8 })
  q.setCenter({ type: null, id: null })
  expect(q.center).toEqual({ type: null, id: null })
  expect(roots).toEqual([{ type: 'note', id: 8 }])
  expect(calls).not.toContain('forget')
  expect(calls).not.toContain('reset')
  expect(written).toEqual([])
})

test('an external centre survives preparation of an empty temporary board', async () => {
  const box = make({
    onRetarget: async () => {
      box.q.setCenter({ type: null, id: null }, false)
    },
  })
  await box.q.syncRoot()
  expect(box.q.center).toEqual({ type: 'quest', id: 5 })
})
