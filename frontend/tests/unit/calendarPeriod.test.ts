import { expect, test } from 'bun:test'

import { dayKey } from '../../src/widgets/activity-calendar/model/day.ts'
import {
  periodBounds,
  periodLabel,
  seasonBounds,
  seasonIndex,
  shiftPeriodAnchor,
  weekEnd,
  weekStart,
} from '../../src/widgets/activity-calendar/model/period.ts'

test('week bounds are Monday–Sunday around the anchor', () => {
  // 2026-11-12 is Thursday
  const d = new Date(2026, 10, 12)
  expect(dayKey(weekStart(d))).toBe('2026-11-09')
  expect(dayKey(weekEnd(d))).toBe('2026-11-15')
  expect(periodBounds('week', d)).toEqual({ from: '2026-11-09', to: '2026-11-15' })
})

test('month / year / season bounds', () => {
  const d = new Date(2026, 10, 12)
  expect(periodBounds('month', d)).toEqual({ from: '2026-11-01', to: '2026-11-30' })
  expect(periodBounds('year', d)).toEqual({ from: '2026-01-01', to: '2026-12-31' })
  expect(periodBounds('season', d)).toEqual({ from: '2026-09-01', to: '2026-11-30' })
  expect(seasonIndex(d)).toBe(3)
  const winter = seasonBounds(new Date(2026, 0, 15))
  expect(dayKey(winter.start)).toBe('2025-12-01')
  expect(dayKey(winter.end)).toBe('2026-02-28')
})

test('shifting a season steps by three months', () => {
  const autumn = new Date(2026, 9, 1)
  const next = shiftPeriodAnchor('season', autumn, 1)
  expect(periodBounds('season', next)).toEqual({ from: '2026-12-01', to: '2027-02-28' })
  const prev = shiftPeriodAnchor('week', new Date(2026, 10, 12), -1)
  expect(periodBounds('week', prev)).toEqual({ from: '2026-11-02', to: '2026-11-08' })
})

test('period labels cover each scale', () => {
  expect(periodLabel('month', '2026-11-01', '2026-11-30')).toMatch(/ноябрь/i)
  expect(periodLabel('year', '2026-01-01', '2026-12-31')).toBe('2026')
  expect(periodLabel('season', '2026-09-01', '2026-11-30')).toBe('осень 2026')
  expect(periodLabel('season', '2025-12-01', '2026-02-28')).toBe('зима 2025–2026')
  expect(periodLabel('week', '2026-11-09', '2026-11-15')).toMatch(/9–15/)
})
