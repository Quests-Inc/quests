import { beforeEach, expect, mock, test } from 'bun:test'

let serverOps: any[] = []
let serverRoots: any[] = []
const calls: string[] = []
let commitResult: any = { applied: 2, created: { 'note:t1': 11 } }
let commitError: any = null

mock.module('@/shared/api/graph.ts', () => ({
  getGraphDraft: async () => ({ ops: serverOps, roots: serverRoots }),
  addGraphDraftOp: async (kind: string, payload: unknown) => {
    const op = { id: serverOps.length + 1, kind, payload }
    serverOps = [...serverOps, op]
    calls.push(`add:${kind}`)
    return op
  },
  commitGraphDraft: async () => {
    calls.push('commit')
    if (commitError) throw commitError
    serverOps = []
    return commitResult
  },
  cancelGraphDraft: async () => {
    calls.push('cancel')
    serverOps = []
  },
  deleteGraphDraftOp: async (id: number, dry = false) => {
    calls.push(`delete:${id}${dry ? ':dry' : ''}`)
    if (dry) return { would_remove: id === 1 ? [1, 2] : [id] }
    serverOps = serverOps.filter((o) => o.id !== id)
  },
  putGraphDraftRoots: async (roots: unknown[]) => {
    calls.push(`roots:${roots.length}`)
    serverRoots = roots
  },
}))
const { createGraphDraft } = await import('@/pages/graph/model/draft.svelte.ts')

function make(center = { type: 'quest', id: 5 }) {
  const state = { center, resets: 0, centers: [] as any[], nodes: [] as any[], saved: new Map<string, any>() }
  const gd = createGraphDraft({
    getCenter: () => state.center,
    setCenter: (c) => {
      state.center = c
      state.centers.push(c)
    },
    getNodes: () => state.nodes,
    getSaved: () => state.saved,
    resetPositions: () => state.resets++,
  })
  return { gd, state }
}

beforeEach(() => {
  serverOps = []
  serverRoots = []
  calls.length = 0
  commitError = null
  commitResult = { applied: 2, created: { 'note:t1': 11 } }
})

test('load и addOp: журнал подтягивается с сервера', async () => {
  const { gd } = make()
  await gd.load()
  expect(gd.draft.ops).toEqual([])
  const op = await gd.addOp('create_note', { fields: { title: 'Новая' } })
  expect(op).toMatchObject({ id: 1, kind: 'create_note' })
  expect(gd.draft.ops).toHaveLength(1)
})

test('save: коммит, позиции переносятся на новые id, перезагрузка карты', async () => {
  const { gd, state } = make()
  await gd.addOp('create_note', { fields: { title: 'Новая' } })
  state.nodes = [
    { key: 'note:t1', type: 'note', x: 10, y: 20, depth: -1, draft: true },
    { key: 'quest:5', type: 'quest', x: 0, y: 0, depth: 0 },
  ]
  await gd.save()
  expect(calls).toContain('commit')
  expect(state.saved.get('note:11')).toEqual({ x: 10, y: 20 })
  expect(state.saved.get('quest:5')).toEqual({ x: 0, y: 0 })
  expect(gd.reloadTick).toBe(1)
  expect(gd.draft.ops).toEqual([])
  expect(gd.committing).toBe(false)
  // новая заметка без связей остаётся отдельным центром
  expect(serverRoots).toEqual([{ type: 'note', id: 11 }])
})

test('save на пустой карте: центр становится первой созданной сущностью', async () => {
  const { gd, state } = make({ type: '', id: 0 } as any)
  await gd.addOp('create_quest', { fields: { title: 'Первый' } })
  commitResult = { applied: 1, created: { 'quest:t1': 7 } }
  await gd.save()
  expect(state.centers).toEqual([{ type: 'quest', id: 7 }])
})

test('save: отказ сервера — проблемы по операциям, журнал открывается', async () => {
  const { gd } = make()
  await gd.addOp('create_note', {})
  gd.open = false
  commitError = Object.assign(new Error('нельзя'), { problems: [{ op: 1, message: 'нет владельца' }] })
  await gd.save()
  expect(gd.problems).toEqual({ 1: 'нет владельца' })
  expect(gd.open).toBe(true)
  expect(gd.reloadTick).toBe(1)
})

test('askDrop: с зависимыми — вопрос, без — сразу; discard сбрасывает позиции', async () => {
  const { gd, state } = make()
  await gd.addOp('create_note', {})
  await gd.addOp('add_ref', {})
  await gd.askDrop({ id: 1 })
  expect(gd.dropAsk).toEqual({ id: 1, ids: [1, 2] })
  await gd.doDrop(1)
  expect(gd.dropAsk).toBeNull()
  expect(gd.draft.ops.map((o: any) => o.id)).toEqual([2])
  await gd.askDrop({ id: 2 })
  expect(gd.draft.ops).toEqual([])
  await gd.addOp('create_note', {})
  gd.cancelAsk = true
  await gd.discard()
  expect(gd.cancelAsk).toBe(false)
  expect(state.resets).toBe(1)
  expect(gd.draft.ops).toEqual([])
})
