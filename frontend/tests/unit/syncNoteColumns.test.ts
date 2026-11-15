import { expect, test } from 'bun:test'

import { middleSourceOffset, syncAnchorFraction, followScrollPosition } from '../../src/shared/lib/syncNoteColumns.ts'

test('middle follows the containing visual line and clamps at both ends', () => {
  const lines = [
    { y: 12, offset: 0 },
    { y: 32, offset: 80 },
    { y: 52, offset: 140 },
  ]
  expect(middleSourceOffset(lines, -10)).toBe(0)
  expect(middleSourceOffset(lines, 32)).toBe(80)
  expect(middleSourceOffset(lines, 49)).toBe(80)
  expect(middleSourceOffset(lines, 500)).toBe(140)
  expect(middleSourceOffset([], 100)).toBe(0)
})

test('sync anchor moves smoothly from top through center to bottom', () => {
  expect(syncAnchorFraction(0, 2000, 500)).toBe(0)
  expect(syncAnchorFraction(250, 2000, 500)).toBe(0.25)
  expect(syncAnchorFraction(1000, 1000, 500)).toBe(0.5)
  expect(syncAnchorFraction(2000, 250, 500)).toBe(0.75)
  expect(syncAnchorFraction(2000, 0, 500)).toBe(1)
})

test('smooth scroll speed is proportional to distance and independent of frame rate', () => {
  const near = followScrollPosition(0, 100, 16)
  const far = followScrollPosition(0, 400, 16)
  expect(far).toBeCloseTo(near * 4)
  expect(followScrollPosition(near, 100, 16)).toBeCloseTo(followScrollPosition(0, 100, 32))
  expect(followScrollPosition(100, 0, 16)).toBeGreaterThan(0)
  expect(followScrollPosition(100, 0, 16)).toBeLessThan(100)
})
