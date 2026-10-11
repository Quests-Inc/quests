import { expect, test } from 'bun:test'

import { borderMarks, hiddenNeighbors, visibleGraph } from '../../src/entities/graph/model/visible.ts'

const kinds = new Set(['questline', 'quest', 'step', 'note', 'attachment'])
const base = {
  root: 'quest:1',
  draftRoots: [],
  hidden: new Set(),
  questStatuses: new Set(['active']),
  shownTypes: kinds,
}

const node = (key, type, extra = {}) => ({ key, type, id: 1, title: key, depth: 1, links: 0, ...extra })

test('квест вне статуса уводит свои шаги, центр и черновик остаются', () => {
  const vis = visibleGraph({
    ...base,
    nodes: [
      node('quest:1', 'quest', { status: 'active', depth: 0 }),
      node('quest:2', 'quest', { status: 'done' }),
      node('step:3', 'step'),
      node('note:t4', 'note', { draft: true, depth: -1 }),
    ],
    edges: [
      { from: 'quest:1', to: 'quest:2', kind: 'reference' },
      { from: 'quest:2', to: 'step:3', kind: 'structure' },
    ],
  })
  expect(vis.nodes.map((n) => n.key).sort()).toEqual(['note:t4', 'quest:1'])
  expect(vis.filteredOut).toBe(1)
  expect(vis.edges).toEqual([])
})

test('скрытый центр остаётся, оторванное от центров — нет', () => {
  const vis = visibleGraph({
    ...base,
    hidden: new Set(['quest:1', 'note:9']),
    nodes: [node('quest:1', 'quest', { status: 'active', depth: 0 }), node('note:9', 'note'), node('note:8', 'note')],
    edges: [{ from: 'note:9', to: 'note:8', kind: 'reference' }],
  })
  expect(vis.nodes.map((n) => n.key)).toEqual(['quest:1'])
})

test('выключенный вид снимается последним и уводит свои вложения', () => {
  const vis = visibleGraph({
    ...base,
    shownTypes: new Set(['quest', 'step', 'attachment', 'questline']),
    nodes: [
      node('quest:1', 'quest', { status: 'active', depth: 0 }),
      node('note:2', 'note'),
      node('attachment:3', 'attachment'),
    ],
    edges: [
      { from: 'quest:1', to: 'note:2', kind: 'reference' },
      { from: 'note:2', to: 'attachment:3', kind: 'structure' },
    ],
  })
  expect(vis.nodes.map((n) => n.key)).toEqual(['quest:1'])
})

test('скрытые соседи собираются по виду и не дублируются', () => {
  const hidden = new Set(['note:2', 'quest:3'])
  const map = hiddenNeighbors(
    [
      { from: 'quest:1', to: 'note:2' },
      { from: 'quest:1', to: 'note:2' },
      { from: 'quest:3', to: 'note:2' },
    ],
    hidden
  )
  expect(map.get('quest:1')).toEqual({ note: ['note:2'] })
  expect(map.has('quest:3')).toBe(false)
})

test('кружки не считают черновик, выключенный вид и квест вне статуса', () => {
  const adj = new Map([['quest:1', { note: ['note:2'] }]])
  expect(borderMarks({ key: 'quest:t1', draft: true }, adj, kinds, () => false)).toEqual([])
  const marks = borderMarks(
    { key: 'quest:1', more: { quest: ['quest:4', 'quest:5'], note: ['note:9'] } },
    adj,
    new Set(['quest', 'step']),
    (key) => key === 'quest:5'
  )
  expect(marks).toEqual([{ type: 'quest', count: 1 }])
})
