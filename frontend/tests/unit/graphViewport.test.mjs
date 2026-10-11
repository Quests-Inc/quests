import { expect, test } from 'bun:test'

import { freeShift, restoreHandPlaced } from '../../src/entities/graph/model/placement.ts'
import { computeTiers, minimapModel } from '../../src/entities/graph/model/viewport.ts'

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
