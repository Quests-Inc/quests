import { expect, test } from 'bun:test'

import { drawableEdges, placedGroups, stepOpenTarget } from '../../src/entities/graph/model/scene.ts'

const card = (key, type, x, extra = {}) => ({
  key,
  type,
  id: 1,
  title: key,
  links: 0,
  depth: 0,
  x,
  y: 0,
  w: 100,
  h: 40,
  ...extra,
})

test('рамка группы обнимает карточки, пустая группа не двигается', () => {
  const groups = [
    { key: 'g', type: 'attachment', owner: 'quest:1', title: 'файлы', count: 1, x: 0, y: 0, w: 10, h: 10 },
  ]
  const [placed] = placedGroups(groups, [card('attachment:1', 'attachment', 200, { group: 'g', y: 100 })])
  expect(placed.x).toBeGreaterThan(100)
  expect(placed.y).toBeGreaterThan(50)
  const [same] = placedGroups(groups, [])
  expect(same).toEqual(groups[0])
})

test('шаг, который ждёт другой шаг, не рисует ещё связь с квестом; выключенный вид не рисуется', () => {
  const nodes = [card('quest:1', 'quest', 0), card('step:1', 'step', 200), card('step:2', 'step', 400)]
  const edges = [
    { from: 'quest:1', to: 'step:2', kind: 'structure' },
    { from: 'step:1', to: 'step:2', kind: 'dependency' },
    { from: 'quest:1', to: 'step:1', kind: 'reference' },
  ]
  const drawn = drawableEdges({
    edges,
    nodes,
    groups: [],
    showEdge: { structure: true, reference: false, dependency: true },
  })
  expect(drawn.map((e) => `${e.kind}:${e.from}->${e.to}`)).toEqual(['dependency:step:1->step:2'])
  expect(drawn[0].d).toContain('M')
})

test('«Открыть» у шага ведёт в квест, черновик никуда не открывается', () => {
  const data = {
    nodes: [
      { key: 'quest:7', id: 7 },
      { key: 'step:3', id: 3 },
    ],
    edges: [{ from: 'quest:7', to: 'step:3', kind: 'structure' }],
  }
  expect(stepOpenTarget({ key: 'step:3', type: 'step', id: 3 }, data)).toEqual(['quest', 7])
  expect(stepOpenTarget({ key: 'note:1', type: 'note', id: 1 }, data)).toEqual(['note', 1])
  expect(stepOpenTarget({ key: 'note:t1', type: 'note', id: 't1', draft: true }, data)).toBeNull()
  expect(stepOpenTarget({ key: 'cluster:quest:1', type: 'cluster', id: 0 }, data)).toBeNull()
})
