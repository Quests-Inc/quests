import { expect, test } from 'bun:test'

import { parseApiDate } from '../../src/shared/lib/time.ts'

test('время без зоны — UTC, в обоих видах: с T и «базовое» с пробелом', () => {
  const iso = parseApiDate('2026-10-17T17:38:00.000000')
  const db = parseApiDate('2026-10-17 17:38:00.000000')
  const z = parseApiDate('2026-10-17T17:38:00Z')
  expect(iso?.toISOString()).toBe('2026-10-17T17:38:00.000Z')
  expect(db?.toISOString()).toBe('2026-10-17T17:38:00.000Z')
  expect(z?.toISOString()).toBe('2026-10-17T17:38:00.000Z')
})

test('зона в строке сохраняется, пустое и мусор — null', () => {
  expect(parseApiDate('2026-10-17T20:38:00+03:00')?.toISOString()).toBe('2026-10-17T17:38:00.000Z')
  expect(parseApiDate('')).toBeNull()
  expect(parseApiDate(null)).toBeNull()
  expect(parseApiDate('вчера')).toBeNull()
})
