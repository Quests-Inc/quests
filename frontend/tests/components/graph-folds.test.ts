import { expect, test } from 'bun:test'

import { createClusterFolds } from '@/pages/graph/model/folds.svelte.ts'

function make() {
  const saved = new Map<string, { x: number; y: number }>()
  const nodes = [
    { key: 'f1', x: 100, y: 100, w: 20, h: 10, group: 'g1' },
    { key: 'f2', x: 140, y: 120, w: 20, h: 10, group: 'g1' },
  ]
  const groups = [{ key: 'g1', type: 'attachment', owner: 'quest:1', x: 0, y: 0 }]
  const folds = createClusterFolds({ saved, getNodes: () => nodes, getGroups: () => groups })
  return { folds, saved }
}

test('expand запоминает место свёрнутого блока; collapse возвращает его туда', () => {
  const { folds, saved } = make()
  folds.expand({ key: 'cluster:quest:1', x: 10, y: 20 })
  expect(folds.expanded.has('cluster:quest:1')).toBe(true)
  saved.set('cluster:quest:1', { x: 999, y: 999 }) // блок сдвинули в раскрытом виде
  folds.collapse('cluster:quest:1')
  expect(folds.expanded.size).toBe(0)
  expect(saved.get('cluster:quest:1')).toEqual({ x: 10, y: 20 })
})

test('без запомненного места сворачивание берёт центр членов группы', () => {
  const { folds, saved } = make()
  folds.disband('quest:1') // не раскрыта: только разложена
  expect(folds.loose.has('quest:1')).toBe(true)
  expect(saved.size).toBe(0)
  const m = make()
  m.folds.expand({ key: 'cluster:quest:1', x: 1, y: 1 })
  m.folds.forgetPositions()
  m.folds.collapse('cluster:quest:1')
  expect(m.saved.get('cluster:quest:1')).toEqual({ x: 120, y: 110 })
})

test('disband раскрытой группы сворачивает её и раскладывает; collapseAll и reset чистят всё', () => {
  const { folds, saved } = make()
  folds.expand({ key: 'cluster:quest:1', x: 5, y: 6 })
  folds.disband('quest:1')
  expect(folds.expanded.size).toBe(0)
  expect(folds.loose.has('quest:1')).toBe(true)
  expect(saved.get('cluster:quest:1')).toEqual({ x: 5, y: 6 })
  folds.expand({ key: 'cluster:note:2', x: 0, y: 0 })
  folds.collapseAll()
  expect(folds.expanded.size + folds.loose.size).toBe(0)
  folds.expand({ key: 'cluster:note:2', x: 0, y: 0 })
  folds.disband('note:9')
  folds.reset()
  expect(folds.expanded.size + folds.loose.size).toBe(0)
})
