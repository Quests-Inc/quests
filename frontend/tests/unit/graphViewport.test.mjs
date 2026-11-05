import { expect, test } from 'bun:test'

import { freeShift, restoreHandPlaced } from '../../src/entities/graph/model/placement.ts'
import {
  computeTiers,
  minimapModel,
  viewportBounds,
  boxInViewport,
  edgeInViewport,
} from '../../src/entities/graph/model/viewport.ts'

test('viewport: screen buffer, partial cards, pan and zoom on a large map', () => {
  const stage = { w: 1000, h: 800 }
  const nodes = Array.from({ length: 10000 }, (_, i) => card(`note:${i}`, i * 500, 400))
  const bounds = viewportBounds({ k: 1, x: 0, y: 0 }, stage)
  expect(nodes.filter((n) => boxInViewport(n, bounds)).length).toBe(3)
  expect(boxInViewport(card('partial', 1280, 400), bounds)).toBe(true)
  expect(boxInViewport(card('outside', 1300, 400), bounds)).toBe(false)
  const moved = viewportBounds({ k: 2, x: -10000, y: -400 }, stage)
  expect(boxInViewport(nodes[10], moved)).toBe(true)
  expect(boxInViewport(nodes[0], moved)).toBe(false)
  expect(boxInViewport(nodes[9999], null)).toBe(true)
  expect(viewportBounds({ k: 1, x: 0, y: 0 }, { w: 0, h: 0 })).toBeNull()
})

test('viewport: retain crossing curves and reject distant edges', () => {
  const bounds = viewportBounds({ k: 1, x: 0, y: 0 }, { w: 1000, h: 800 })
  expect(edgeInViewport({ sx: -2000, sy: 400, ex: 3000, ey: 400, fx: 1, fy: 0 }, bounds)).toBe(true)
  expect(edgeInViewport({ sx: 9000, sy: 9000, ex: 9200, ey: 9200, fx: 0, fy: 1 }, bounds)).toBe(false)
  expect(edgeInViewport({ sx: 9000, sy: 9000, ex: 9200, ey: 9200, fx: 0, fy: 1 }, null)).toBe(true)
  // A long horizontal edge above the viewport must not count as crossing it.
  expect(edgeInViewport({ sx: -9000, sy: -1000, ex: 9000, ey: -1000, fx: 1, fy: 0 }, bounds)).toBe(false)
})

const card = (key, x, y, extra = {}) => ({
  key,
  type: 'quest',
  x,
  y,
  w: 100,
  h: 60,
  links: 0,
  depth: 0,
  title: key,
  id: 1,
  ...extra,
})

test('computeTiers: далеко за экраном — lite, мелкая — lite, средняя — title, крупная — полная', () => {
  const stage = { w: 1000, h: 800 }
  const cam = { k: 1, x: 0, y: 0 }
  const nodes = [
    card('quest:far', 9000, 9000),
    card('quest:tiny', 100, 100, { w: 20, h: 20 }),
    card('quest:mid', 300, 100, { w: 90, h: 60 }),
    card('quest:big', 600, 400, { w: 300, h: 260 }),
    card('step:plain', 600, 100, { type: 'step' }),
  ]
  const out = computeTiers({ nodes, cam, stage, ready: true, prev: new Map() })
  expect(out.get('quest:far')).toBe('lite')
  expect(out.get('quest:tiny')).toBe('lite')
  expect(out.get('quest:mid')).toBe('title')
  expect(out.has('quest:big')).toBe(false)
  expect(out.has('step:plain')).toBe(false)
})

test('computeTiers: не готово — пусто; без изменений возвращает прежний Map; редактируемая — полная', () => {
  const stage = { w: 1000, h: 800 }
  const cam = { k: 1, x: 0, y: 0 }
  expect(computeTiers({ nodes: [card('quest:1', 0, 0)], cam, stage, ready: false, prev: new Map() }).size).toBe(0)
  const nodes = [card('quest:1', 9000, 0)]
  const first = computeTiers({ nodes, cam, stage, ready: true, prev: new Map() })
  expect(computeTiers({ nodes, cam, stage, ready: true, prev: first })).toBe(first)
  expect(computeTiers({ nodes, cam, stage, ready: true, editingKey: 'quest:1', prev: new Map() }).size).toBe(0)
})

test('minimapModel: пусто без узлов; рамка вида считается по камере', () => {
  const stage = { w: 400, h: 300 }
  expect(minimapModel({ nodes: [], ready: true, cam: { k: 1, x: 0, y: 0 }, stage })).toBeNull()
  const m = minimapModel({ nodes: [card('quest:1', 0, 0)], ready: true, cam: { k: 2, x: -100, y: -50 }, stage })
  expect(m.view).toEqual({ x: 50, y: 25, w: 200, h: 150 })
  expect(m.w).toBeLessThanOrEqual(170)
  expect(m.h).toBeLessThanOrEqual(120)
})

test('restoreHandPlaced: руками поставленные карточки возвращаются на место', () => {
  const nodes = [card('quest:1', 0, 0), card('quest:2', 10, 10)]
  restoreHandPlaced(nodes, new Map([['quest:2', { x: 500, y: 600 }]]), [])
  expect(nodes[0]).toMatchObject({ x: 0, y: 0 })
  expect(nodes[1]).toMatchObject({ x: 500, y: 600 })
})

test('freeShift: свободный блок не двигается, занятый уходит на ближайшее свободное место', () => {
  const a = card('attachment:1', 0, 0, { group: 'g' })
  const blocker = card('quest:9', 0, 0)
  expect(freeShift([a], { key: 'g' }, [a], [])).toEqual({ dx: 0, dy: 0 })
  const shift = freeShift([a, blocker], { key: 'g' }, [a], [])
  expect(shift.dx).toBe(0)
  // рамка блока выше, чем ниже карточки, поэтому ближе всего свободно сверху
  expect(Math.abs(shift.dy)).toBeGreaterThan(100)
})
