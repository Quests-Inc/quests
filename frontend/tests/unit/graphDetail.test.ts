import { expect, test } from 'bun:test'

import { revealCardBody } from '../../src/widgets/graph-canvas/model/cardReveal.ts'
import { imageLevel, imagePixels, noteImageLevel } from '../../src/widgets/graph-canvas/model/imageQuality.ts'
import { outlinePath } from '../../src/widgets/graph-canvas/model/silhouette.ts'
test('note outlines preserve blanks and relative line lengths with bounded work', () => {
  const path = outlinePath('short\n\na much longer line')
  expect(path.match(/M2/g)?.length).toBe(2)
  expect(path).toBe(outlinePath('short\n\na much longer line'))
  expect((outlinePath('line\n'.repeat(10000)).match(/M2/g) || []).length).toBeLessThanOrEqual(32)
  expect(outlinePath('')).toBe('')
})
test('image resolution follows scale, density and viewport intersection', () => {
  const n = { x: 100, y: 100, w: 100, h: 100 },
    cam = { x: 0, y: 0, k: 1 },
    view = { w: 1000, h: 800 }
  expect(imagePixels(n, cam, view)).toBe(100)
  expect(imagePixels(n, cam, view, 2)).toBe(200)
  expect(imagePixels(n, { ...cam, k: 2 }, view)).toBe(200)
  expect(imagePixels({ ...n, x: 1000 }, cam, view)).toBeLessThan(100)
  expect(imagePixels({ ...n, x: 2000 }, cam, view)).toBeGreaterThan(0)
  expect(imageLevel(100)).toBe(128)
  expect(imageLevel(2000)).toBe(2048)
  expect(imageLevel(4000)).toBe(0)
  const note = { x: 0, y: 0, w: 400, h: 300, type: 'note' }
  expect(noteImageLevel(note, 1)).toBe(noteImageLevel(note, 1))
  expect(noteImageLevel(note, 1)).toBe(imageLevel(400))
  expect(noteImageLevel(note, 2)).toBe(imageLevel(800))
})

test('revealCardBody measures a card that is already showing and defers a new one', () => {
  const shown: boolean[] = []
  let measured = 0
  revealCardBody({
    defer: true,
    enabled: true,
    shown: true,
    show: (value) => shown.push(value),
    measure: () => {
      measured++
    },
  })
  expect(measured).toBe(1)
  expect(shown).toEqual([])
  revealCardBody({
    defer: true,
    enabled: false,
    shown: true,
    show: (value) => shown.push(value),
    measure: () => {
      measured++
    },
  })
  expect(shown).toEqual([false])
})
