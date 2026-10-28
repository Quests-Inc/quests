import { expect, test } from 'bun:test'

import type { GraphNodeView } from '@/entities/graph'
import { createCardResize, MAX_CARD, MIN_CARD } from '@/widgets/graph-canvas/model/resize.svelte.ts'

const node = (): GraphNodeView => ({
  key: 'note:1',
  type: 'note',
  id: 1,
  title: 'n',
  links: 0,
  depth: 0,
  x: 100,
  y: 50,
  w: 200,
  h: 100,
})

function setup(k = 1) {
  const sizes = new Map<string, { w: number; h: number } | null>()
  const saved = new Map<string, { x: number; y: number }>()
  let touched = 0
  let remeasured = 0
  const r = createCardResize({
    getK: () => k,
    touchView: () => touched++,
    getSaved: () => saved,
    setSize: (key, s) => void sizes.set(key, s),
    remeasure: () => remeasured++,
  })
  const down = (n: GraphNodeView, x: number, y: number) =>
    r.start(
      { button: 0, clientX: x, clientY: y, stopPropagation() {}, preventDefault() {} } as unknown as PointerEvent,
      n
    )
  const move = (x: number, y: number) =>
    window.dispatchEvent(Object.assign(new Event('pointermove'), { clientX: x, clientY: y }))
  const up = () => window.dispatchEvent(new Event('pointerup'))
  return {
    r,
    sizes,
    saved,
    down,
    move,
    up,
    get touched() {
      return touched
    },
    get remeasured() {
      return remeasured
    },
  }
}

test('за уголок карточка растёт, левый верхний угол стоит, размер и место запоминаются', () => {
  const t = setup()
  const n = node()
  t.down(n, 0, 0)
  t.move(60, 40)
  expect([n.sw, n.sh, n.w, n.h]).toEqual([260, 140, 260, 140])
  // The centre moved by half the growth: left = x - w/2 = 0 and top = y - h/2 = 0 as before.
  expect([n.x - n.w / 2, n.y - n.h / 2]).toEqual([0, 0])
  t.up()
  expect(t.sizes.get('note:1')).toEqual({ w: 260, h: 140 })
  expect(t.saved.get('note:1')).toEqual({ x: n.x, y: n.y })
})

test('масштаб поля учитывается, размер зажат между минимумом и максимумом', () => {
  const t = setup(2)
  const n = node()
  t.down(n, 0, 0)
  t.move(100, 0) // 100 экранных пикселей при зуме 2 — 50 в мире
  expect(n.w).toBe(250)
  t.move(-10000, -10000)
  expect([n.w, n.h]).toEqual([MIN_CARD.w, MIN_CARD.h])
  t.move(100000, 100000)
  expect([n.w, n.h]).toEqual([MAX_CARD.w, MAX_CARD.h])
  t.up()
})

test('щелчок без движения ничего не запоминает; сброс возвращает размер по содержимому', () => {
  const t = setup()
  const n = node()
  t.down(n, 5, 5)
  t.up()
  expect(t.sizes.size).toBe(0)
  n.sw = 300
  n.sh = 200
  t.r.reset(n)
  expect([n.sw, n.sh]).toEqual([undefined, undefined])
  expect(t.sizes.get('note:1')).toBeNull()
  expect(t.remeasured).toBe(1)
})
