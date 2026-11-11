import { expect, test } from 'bun:test'

import { SLIDE_MS, slideTab } from '../../src/app/model/slideTab.ts'

test('a forward tab enters from the right and a backward one from the left', () => {
  const forward = slideTab(null, { dir: 1 })
  const back = slideTab(null, { dir: -1, out: true })
  expect(forward.duration).toBe(SLIDE_MS)
  expect(forward.css(0)).toBe('transform: translateX(100%)')
  expect(forward.css(1)).toBe('transform: translateX(0%)')
  expect(back.css(0)).toBe('transform: translateX(100%)')
})
