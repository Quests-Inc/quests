import { expect, test } from 'bun:test'

import { cameraOnRect } from '@/entities/graph'
import { CONTENT_DWELL_MS, planContent } from '@/widgets/graph-canvas/model/contentDwell.ts'

test('тяжёлый рендер стартует на неподвижной камере и не во время жеста', () => {
  const moving = planContent({
    now: 0,
    idleSince: 0,
    pointer: false,
    camMoved: true,
    visible: ['note:1'],
    mounted: ['note:1'],
    clocks: new Map(),
    warm: new Set(),
  })
  expect(moving.idle).toBe(false)
  expect(moving.warm.size).toBe(0)
  expect(CONTENT_DWELL_MS).toBe(0)

  const ready = planContent({
    now: 0,
    idleSince: 0,
    pointer: false,
    camMoved: false,
    visible: ['note:1', 'note:2'],
    mounted: ['note:1', 'note:2'],
    clocks: moving.clocks,
    warm: moving.warm,
  })
  expect(ready.idle).toBe(true)
  expect([...ready.warm].sort()).toEqual(['note:1', 'note:2'])

  const left = planContent({
    now: 10,
    idleSince: 0,
    pointer: false,
    camMoved: false,
    visible: [],
    mounted: [],
    clocks: ready.clocks,
    warm: ready.warm,
  })
  expect(left.warm.size).toBe(0)
})

test('камера зоны вписывает прямоугольник и не перескакивает к мелкому масштабу', () => {
  const cam = cameraOnRect({ x: 0, y: 0, w: 400, h: 200 }, { w: 800, h: 600 }, 1)
  expect(cam).not.toBeNull()
  expect(cam!.k).toBeLessThanOrEqual(2.5)
  expect(cam!.x + 200 * cam!.k).toBeCloseTo(400)
  expect(cam!.y + 100 * cam!.k).toBeCloseTo(300)
})
