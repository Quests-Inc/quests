import { expect, test } from 'bun:test'

import { setPendingFragment, takePendingFragment, validNoteHash } from '@/shared/lib/noteFragment.ts'

test('a note hash is a key or a table row', () => {
  expect(validNoteHash('#light')).toBe(true)
  expect(validNoteHash('#spell/classes')).toBe(true)
  expect(validNoteHash('#')).toBe(false)
  expect(validNoteHash('#Spell')).toBe(false)
})

test('the pending fragment is taken once for the note that asked', () => {
  setPendingFragment(82, 'light')
  expect(takePendingFragment(88)).toBe('')
  expect(takePendingFragment(82)).toBe('light')
  expect(takePendingFragment(82)).toBe('')
})
