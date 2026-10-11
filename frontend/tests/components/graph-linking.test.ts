import { expect, test } from 'bun:test'

import { createLinkMode } from '@/widgets/graph-canvas/model/linking.svelte.ts'

const nodes = new Map<string, any>([
  ['quest:1', { key: 'quest:1', type: 'quest', x: 10, y: 20 }],
  ['quest:2', { key: 'quest:2', type: 'quest', x: 0, y: 0 }],
  ['note:3', { key: 'note:3', type: 'note', x: 0, y: 0 }],
  ['note:4', { key: 'note:4', type: 'note', x: 0, y: 0 }],
])

function make(edges: any[] = []) {
  const ops: [string, any][] = []
  const drops: number[] = []
  let started = 0
  const lk = createLinkMode({
    getNodeMap: () => nodes,
    getEdges: () => edges,
    addOp: async (k, p) => void ops.push([k, p]),
    askDrop: async (op) => void drops.push(op.id),
    onStart: () => started++,
  })
  return { lk, ops, drops, started: () => started }
}

test('start ставит якорь и курсор на карточку, cancel сбрасывает', () => {
  const { lk, started } = make()
  lk.start('to', { key: 'quest:1' })
  expect(lk.linking).toEqual({ mode: 'to', anchor: 'quest:1' })
  expect(lk.cursor).toEqual({ x: 10, y: 20 })
  expect(started()).toBe(1)
  lk.cancel()
  expect(lk.linking).toBeNull()
})

test('ссылка «к»: add_ref от якоря к цели; цель нельзя выбрать вне режима и себя', async () => {
  const { lk, ops } = make()
  expect(lk.canPick({ key: 'quest:2', type: 'quest' })).toBe(false)
  lk.start('to', { key: 'quest:1' })
  expect(lk.canPick({ key: 'quest:1', type: 'quest' })).toBe(false)
  await lk.finish({ key: 'quest:2', type: 'quest' })
  expect(ops).toEqual([['add_ref', { from: 'quest:1', to: 'quest:2' }]])
  expect(lk.linking).toBeNull()
})

test('ссылка «из»: направление обратное; дубликат не пишется', async () => {
  const { lk, ops } = make([{ kind: 'reference', from: 'note:3', to: 'quest:1' }])
  lk.start('from', { key: 'quest:1' })
  await lk.finish({ key: 'note:3', type: 'note' })
  expect(ops).toEqual([])
  lk.start('from', { key: 'quest:1' })
  await lk.finish({ key: 'note:4', type: 'note' })
  expect(ops).toEqual([['add_ref', { from: 'note:4', to: 'quest:1' }]])
})

test('attach: заметка уходит под заметку (set_parent), прочее — attach; уже прикреплённое пропускается', async () => {
  const { lk, ops } = make([{ kind: 'structure', from: 'note:4', to: 'quest:1' }])
  lk.start('attach', { key: 'note:3' })
  await lk.finish({ key: 'note:4', type: 'note' })
  expect(ops[0][0]).toBe('set_parent')
  expect(ops[0][1]).toMatchObject({ id: 'note:3', parent: 'note:4' })
  lk.start('attach', { key: 'quest:1' })
  await lk.finish({ key: 'note:4', type: 'note' })
  expect(ops).toHaveLength(1)
  lk.start('attach', { key: 'quest:2' })
  await lk.finish({ key: 'note:3', type: 'note' })
  expect(ops[1]).toEqual(['attach', { item: 'quest:2', owner: 'note:3' }])
})

test('unlinkRef: своя в черновике снимается удалением шага журнала, сохранённая — remove_ref', async () => {
  const { lk, ops, drops } = make([
    { kind: 'reference', from: 'note:3', to: 'quest:1', draft: 'add', op: 7 },
    { kind: 'reference', from: 'note:3', to: 'quest:2' },
  ])
  await lk.unlinkRef({ key: 'note:3' }, 'quest:1')
  expect(drops).toEqual([7])
  await lk.unlinkRef({ key: 'note:3' }, 'quest:2')
  expect(ops).toEqual([['remove_ref', { from: 'note:3', to: 'quest:2' }]])
})
