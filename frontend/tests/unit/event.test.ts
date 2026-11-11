import { expect, test } from 'bun:test'

import { eventPhrase, offsetLabel, offsetSeconds, secondsToOffset } from '../../src/shared/lib/event.ts'

test('offset converts days, hours and minutes both ways', () => {
  expect(offsetSeconds('1', '2', '3')).toBe(86400 + 2 * 3600 + 3 * 60)
  expect(offsetSeconds('no', -4, '')).toBe(0)
  expect(secondsToOffset(offsetSeconds(1, 2, 3))).toEqual({ days: '1', hours: '2', minutes: '3' })
  expect(secondsToOffset(0)).toEqual({ days: '', hours: '', minutes: '' })
  expect(offsetLabel(90060)).toBe('1 дн 1 ч 1 мин')
})

test('event phrase names the edge, the direction and the title', () => {
  expect(eventPhrase({ event_id: 1, event_title: 'День', event_edge: 'end' })).toBe('в конце «День»')
  expect(eventPhrase({ event_id: 1, event_edge: 'start', event_offset_seconds: 3600 })).toBe('за 1 ч до начала события')
  expect(
    eventPhrase({ event_id: 1, event_title: 'День', event_edge: 'end', event_dir: 'after', event_offset_seconds: 120 })
  ).toBe('через 2 мин после конца «День»')
  expect(eventPhrase({})).toBe('')
})
