import { expect, test } from 'bun:test'

import { arrangeView } from '../../src/entities/graph/model/arrange.ts'

const node = (key, over = {}) => ({
  key,
  type: 'quest',
  id: 1,
  title: key,
  x: 0,
  y: 0,
  w: 100,
  h: 40,
  depth: 0,
  ...over,
})
const view = () => ({
  nodes: [
    node('a'),
    node('b', { depth: 1 }),
    node('free', { depth: -1 }),
    node('placed', { depth: -1, pos: { x: 500, y: 600 } }),
  ],
  edges: [{ from: 'a', to: 'b', kind: 'structure' }],
})

test('несвязанные: с позицией — на ней, без — колонкой слева от карты', () => {
  const v = view()
  arrangeView(v, 'layers', { loose: new Set(), saved: new Map() })
  const placed = v.nodes.find((n) => n.key === 'placed')
  expect([placed.x, placed.y]).toEqual([500, 600])
  const free = v.nodes.find((n) => n.key === 'free')
  const left = Math.min(...v.nodes.filter((n) => n.depth >= 0).map((n) => n.x - n.w / 2))
  expect(free.x).toBe(left - 260)
})

test('руками поставленные карточки возвращаются на сохранённое место', () => {
  const v = view()
  arrangeView(v, 'down', { loose: new Set(), saved: new Map([['b', { x: 42, y: -7 }]]) })
  const b = v.nodes.find((n) => n.key === 'b')
  expect([b.x, b.y]).toEqual([42, -7])
})

test('любая раскладка отдаёт группы и ставит все связанные карточки', () => {
  for (const mode of ['tree', 'down', 'radial', 'layers', 'force']) {
    const v = view()
    const groups = arrangeView(v, mode, { loose: new Set(), saved: new Map() })
    expect(Array.isArray(groups)).toBe(true)
    for (const n of v.nodes) {
      expect(Number.isFinite(n.x)).toBe(true)
      expect(Number.isFinite(n.y)).toBe(true)
    }
  }
})

test('вне дерева у карточек сбрасывается принадлежность группе', () => {
  const v = view()
  v.nodes[0].group = 'g'
  arrangeView(v, 'layers', { loose: new Set(), saved: new Map() })
  expect(v.nodes[0].group).toBeUndefined()
})
