import { expect, test } from 'bun:test'

import { arrowHead, clampK, edgePath, sidesOf, spreadEnds } from '../../src/entities/graph/model/geometry.ts'

const box = (x, y, key) => ({ x, y, w: 100, h: 40, key })

test('sidesOf: далеко по x — горизонтальное ребро, иначе вертикальное', () => {
  expect(sidesOf(box(0, 0), box(300, 10))).toMatchObject({ vertical: false, dir: 1, sa: 'R', sb: 'L' })
  expect(sidesOf(box(300, 0), box(0, 10))).toMatchObject({ vertical: false, dir: -1, sa: 'L', sb: 'R' })
  expect(sidesOf(box(0, 0), box(10, 200))).toMatchObject({ vertical: true, dir: 1, sa: 'B', sb: 'T' })
  expect(sidesOf(box(0, 200), box(10, 0))).toMatchObject({ vertical: true, dir: -1, sa: 'T', sb: 'B' })
})

test('edgePath: начинается на грани карточки a', () => {
  const horizontal = edgePath(box(0, 0), box(300, 0))
  expect(horizontal.sx).toBe(50)
  expect(horizontal.sy).toBe(0)
  expect(horizontal.d.startsWith('M50 0 C')).toBe(true)
  const vertical = edgePath(box(0, 0), box(0, 200), 5)
  expect(vertical.sx).toBe(5)
  expect(vertical.sy).toBe(20)
})

test('spreadEnds: рёбра с одной стороны получают разные точки, по порядку другого конца', () => {
  const a = box(0, 0, 'a')
  const items = [
    { e: 1, a, b: box(300, 100, 'b') },
    { e: 2, a, b: box(300, -100, 'c') },
  ]
  spreadEnds(items)
  expect(items[0].oa).not.toBe(items[1].oa)
  // тот, кто ведёт выше (меньший y), выходит выше
  expect(items[1].oa).toBeLessThan(items[0].oa)
})

test('clampK: не выходит за пределы, но не прыгает с карты, уже вне диапазона', () => {
  expect(clampK(10, 1)).toBe(2.5)
  expect(clampK(0.01, 1)).toBe(0.2)
  expect(clampK(0.1, 0.05)).toBe(0.1)
})

test('arrowHead: треугольник стрелки на конце ребра, как маркер (длина 7 толщин, кончик чуть за концом)', () => {
  const right = edgePath(box(0, 0), box(300, 0))
  expect(right).toMatchObject({ ex: 250, ey: 0, fx: 1, fy: 0 })
  expect(arrowHead(right, 10)).toBe('M257 0L187 35L187 -35Z')
  const down = edgePath(box(0, 0), box(0, 200))
  expect(down).toMatchObject({ ex: 0, ey: 180, fx: 0, fy: 1 })
  expect(arrowHead(down, 10)).toMatch(/^M0 187L/)
})
