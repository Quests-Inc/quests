import { expect, test } from 'bun:test'

import { planHide } from '../../src/entities/graph/model/hide.ts'

const nodes = [
  { key: 'quest:1' },
  { key: 'step:2' },
  { key: 'note:3' },
  { key: 'note:t9', draft: 'add' },
  { key: 'quest:5' },
]
const edges = [
  { from: 'quest:1', to: 'step:2' },
  { from: 'quest:1', to: 'note:3' },
]
const base = { nodes, edges, rootKey: 'quest:1', hidden: new Set(['x']), roots: [{ type: 'quest', id: 5 }] }

test('скрывается только то, что связано с остающимся; прежние скрытые сохраняются', () => {
  const p = planHide({ ...base, keys: ['step:2', 'quest:5'] })
  expect([...p.hidden].sort()).toEqual(['step:2', 'x'])
  expect(p.roots).toEqual([])
  expect(p.rootsChanged).toBe(true)
})

test('центр карты не скрывается, карточка черновика пропускается и считается', () => {
  const p = planHide({ ...base, keys: ['quest:1', 'note:t9'] })
  expect(p.clearAll).toBe(false)
  expect(p.hidden).toEqual(new Set(['x']))
  expect(p.skippedDraft).toBe(1)
})

test('снят центр и не осталось настоящих карточек — очистка карты', () => {
  const p = planHide({ ...base, nodes: [{ key: 'quest:1' }, { key: 'note:t9', draft: 1 }], keys: ['quest:1'] })
  expect(p.clearAll).toBe(true)
})

test('неизвестный ключ и карточка без связей просто исчезают', () => {
  const p = planHide({ ...base, keys: ['nope:1', 'note:3'] })
  expect([...p.hidden].sort()).toEqual(['note:3', 'x'])
})
