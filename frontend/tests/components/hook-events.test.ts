import { describe, expect, test } from 'bun:test'

import { eventWords, expandEvents } from '@/entities/hook/model/catalog.svelte.ts'
import { displayEvents, newHookDraft } from '@/entities/hook/model/events.ts'
import { statusWord } from '@/shared/lib/questStatus.ts'

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

  test('aliases unfold into kinds; a whole family reads as one phrase, the rest by title', () => {
    const aliases = { note: ['note_created', 'note_updated'], complete: ['quest_completed'] }
    expect(expandEvents(['note', 'complete', 'quest_completed'], aliases)).toEqual([
      'note_created',
      'note_updated',
      'quest_completed',
    ])
    const events = [
      { kind: 'note_created', family: 'note', title: 'заметка создана', fields: [] },
      { kind: 'note_updated', family: 'note', title: 'заметка изменена', fields: [] },
      { kind: 'quest_completed', family: 'quest', title: 'квест выполнен', fields: [] },
      { kind: 'quest_failed', family: 'quest', title: 'квест провален', fields: [] },
    ]
    expect(eventWords(['note_created', 'note_updated', 'quest_completed'], events)).toEqual([
      'все события: заметка',
      'квест выполнен',
    ])
    expect(eventWords(['mystery'], events)).toEqual(['mystery'])
    expect(eventWords(['x'], [])).toEqual(['x'])
  })

  test('a quest status is shown as a word, the code stays for the unknown', () => {
    expect(statusWord('expired')).toBe('просрочен')
    expect(statusWord('weird')).toBe('weird')
    expect(statusWord(null)).toBe('')
  })
})
