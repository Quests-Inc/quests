import { expect, test } from 'bun:test'

import {
  clearRevealedNotes,
  rememberRevealed,
  revealedBody,
  revealedEntries,
} from '@/shared/lib/privateReveal.svelte.ts'

test('a revealed note stays while the map is open and goes away when the map does', () => {
  clearRevealedNotes()
  expect(revealedBody('note:4')).toBeUndefined()
  rememberRevealed('note:4', 'секрет')
  expect(revealedBody('note:4')).toBe('секрет')
  expect(revealedEntries()).toEqual([['note:4', 'секрет']])
  clearRevealedNotes()
  expect(revealedBody('note:4')).toBeUndefined()
})
