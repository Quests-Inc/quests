import { expect, test } from 'bun:test'

import { imagePixels } from '../../src/widgets/graph-canvas/model/imageQuality.ts'
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
})
