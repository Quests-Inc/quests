import { expect, test } from 'bun:test'

import type { EventRead } from '../../src/shared/api/journal.gen.ts'
import {
  buildBlocks,
  buildDayLoad,
  eventsByDayMap,
  questsByDayMap,
} from '../../src/widgets/activity-calendar/model/dayLoad.ts'

test('quests and events land on their calendar days', () => {
  const quests = [{ id: 1, title: 'A', status: 'active', significance: 'common', deadline_at: '2026-11-12T15:00:00' }]
  const events = [
    {
      id: 2,
      title: 'E',
      starts_at: '2026-11-12T10:00:00',
      ends_at: '2026-11-13T10:00:00',
    } as EventRead,
  ]
  expect(questsByDayMap(quests)['2026-11-12']?.[0]?.id).toBe(1)
  expect(eventsByDayMap(events)['2026-11-12']?.length).toBe(1)
  expect(eventsByDayMap(events)['2026-11-13']?.length).toBe(1)
})

test('month blocks cover the period; week is a single row', () => {
  const empty = {}
  const months = buildBlocks('month', '2026-11-01', '2026-11-30', empty)
  expect(months).toHaveLength(1)
  expect(months[0].cells.filter(Boolean).length).toBe(30)
  const week = buildBlocks('week', '2026-11-09', '2026-11-15', empty)
  expect(week).toHaveLength(1)
  expect(week[0].cells).toHaveLength(7)
})

test('day load puts an event on the board for its start day', () => {
  const events = [
    {
      id: 7,
      title: 'Meet',
      color: '#5a8a9a',
      icon: 'calendar',
      starts_at: '2026-11-12T10:00:00',
      ends_at: '2026-11-12T11:00:00',
    } as EventRead,
  ]
  const load = buildDayLoad('2026-11-12', [], events)
  expect(load.eventRows).toHaveLength(1)
  expect(load.eventRows[0].id).toBe(7)
  expect(buildDayLoad('2026-11-13', [], events).eventRows).toHaveLength(0)
})
