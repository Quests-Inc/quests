import { expect, test } from 'bun:test'

import type { EventRead } from '../../src/shared/api/journal.gen.ts'
import {
  buildBarCaption,
  clipToDay,
  dayBounds,
  dayKey,
  eventBarCaption,
  questWindow,
} from '../../src/widgets/activity-calendar/model/day.ts'

const at = (h: number, m = 0) => new Date(2026, 10, 12, h, m, 0, 0)

test('a quest window ends at the deadline and opens duration earlier', () => {
  const deadline = at(15).toISOString()
  const point = questWindow({ deadline_at: deadline })
  expect(point?.start.getTime()).toBe(point?.end.getTime())
  const span = questWindow({ deadline_at: deadline, duration_seconds: 3600 })
  expect(span && span.end.getTime() - span.start.getTime()).toBe(3600 * 1000)
  expect(questWindow({})).toBeNull()
})

test('clipping keeps the part of a window that falls on the day', () => {
  const { start, end } = dayBounds('2026-11-12')
  expect(dayKey(start)).toBe('2026-11-12')
  const inside = clipToDay({ start: at(10), end: at(12) }, start, end)
  expect(inside?.point).toBe(false)
  expect(clipToDay({ start: at(10), end: at(9) }, start, end)).toBeNull()
})

test('bar captions name an all-day event and a finished quest', () => {
  const ev = { title: 'Sssss', all_day: true } as EventRead
  const win = { start: at(0), end: new Date(2026, 10, 13, 0, 0, 0, 0), finish: null }
  expect(eventBarCaption(ev, win, false, true)).toBe('Sssss · весь день')
  expect(buildBarCaption({ title: 'Квест' }, win, false, true, 'completed')).toContain('выполнен')
})
