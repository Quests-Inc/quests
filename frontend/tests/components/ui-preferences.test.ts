import { expect, test } from 'bun:test'

import { uiPreferences } from '@/shared/model/uiPreferences.svelte.ts'

test('note history visibility is persisted and can be restored', () => {
  uiPreferences.hideNoteHistory = true
  expect(uiPreferences.hideNoteHistory).toBe(true)
  expect(localStorage.getItem('quests.ui.hideNoteHistory')).toBe('true')
  uiPreferences.hideNoteHistory = false
  expect(uiPreferences.hideNoteHistory).toBe(false)
  expect(localStorage.getItem('quests.ui.hideNoteHistory')).toBe('false')
})

test('history icons can be hidden independently of the history row', () => {
  uiPreferences.hideNoteHistory = false
  uiPreferences.hideNoteHistoryIcons = true
  expect(uiPreferences.hideNoteHistory).toBe(false)
  expect(uiPreferences.hideNoteHistoryIcons).toBe(true)
  expect(localStorage.getItem('quests.ui.hideNoteHistoryIcons')).toBe('true')
  uiPreferences.hideNoteHistoryIcons = false
  expect(localStorage.getItem('quests.ui.hideNoteHistoryIcons')).toBe('false')
})
