import { expect, test } from 'bun:test'

import { parentKeyOf } from '@/pages/notes/model/parentKey.ts'

const note = { id: 12, parent_id: 4 }

test('the loaded editor parent wins over the draft and the server', () => {
  expect(parentKeyOf(note, 12, { loadedId: 12, parentId: '' }, '9')).toBe('root')
})

test('a draft is used until the editor has loaded this note', () => {
  expect(parentKeyOf(note, 12, { loadedId: 7, parentId: '3' }, '9')).toBe('9')
  expect(parentKeyOf(note, 8, { loadedId: 8, parentId: '3' }, undefined)).toBe('4')
})
