import { expect, test } from 'bun:test'

import { diffLayoutEntities, layoutEntities } from '../../src/entities/graph/model/layoutDiff.ts'

test('manifest compares entity keys, ignoring movement and renamed titles', () => {
  const before = {
    entities: [
      { key: 'quest:1', title: 'Старое имя' },
      { key: 'note:2', title: 'Убрано' },
    ],
  }
  const after = {
    entities: [
      { key: 'quest:1', title: 'Новое имя' },
      { key: 'note:3', title: 'Добавлено' },
      { key: 'cluster:note:3', title: 'Группа' },
      { key: 'note:t1', title: 'Черновик' },
    ],
  }
  expect(diffLayoutEntities(before, after)).toEqual({
    added: [{ key: 'note:3', title: 'Добавлено' }],
    removed: [{ key: 'note:2', title: 'Убрано' }],
  })
  expect(layoutEntities({ entities: [], positions: { 'note:2': {} } })).toEqual([])
})

test('legacy layouts infer only actual entities, excluding hidden cards and clusters', () => {
  expect(
    layoutEntities({
      positions: { 'note:2': {}, 'note:3': {}, 'cluster:note:2': {}, 'note:t1': {} },
      hidden: ['note:3'],
      center: { type: 'quest', id: 1 },
    })
  ).toEqual([
    { key: 'note:2', title: 'note:2' },
    { key: 'quest:1', title: 'quest:1' },
  ])
})
