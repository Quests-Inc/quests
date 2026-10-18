import { describe, expect, test } from 'bun:test'

import { displayEvents, newHookDraft } from '@/entities/hook/model/events.ts'

describe('hook events helpers', () => {
  test('displayEvents prefers events_raw', () => {
    expect(displayEvents({ events_raw: ['complete'], events: ['quest_completed'] })).toEqual(['complete'])
    expect(displayEvents({ events: ['quest_completed'] })).toEqual(['quest_completed'])
    expect(displayEvents({})).toEqual([])
  })

  test('newHookDraft seeds defaults', () => {
    const d = newHookDraft({ events: ['step'] })
    expect(d.events).toEqual(['step'])
    expect(d.enabled).toBe(true)
    expect(d.thisQuest).toBe(true)
    expect(d.localId).toBeLessThan(0)
  })
})
