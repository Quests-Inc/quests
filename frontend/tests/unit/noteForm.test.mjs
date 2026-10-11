import { expect, test } from 'bun:test'

import { noteFormFromRow, noteFormsEqual } from '../../src/entities/note/model/form.ts'

test('noteFormFromRow: пустые поля и флаги приводятся к форме', () => {
  expect(noteFormFromRow({})).toEqual({
    title: '',
    description: '',
    parentId: '',
    pinned: false,
    isReadme: false,
    isPrivate: false,
    isCategory: false,
  })
  expect(
    noteFormFromRow({
      title: 'Т',
      description: 'Д',
      parent_id: 7,
      pinned: true,
      is_readme: true,
      is_private: true,
      is_category: true,
    })
  ).toEqual({
    title: 'Т',
    description: 'Д',
    parentId: '7',
    pinned: true,
    isReadme: true,
    isPrivate: true,
    isCategory: true,
  })
})

test('noteFormsEqual: любое поле различает формы', () => {
  const a = noteFormFromRow({ title: 'Т', parent_id: 1 })
  expect(noteFormsEqual(a, { ...a })).toBe(true)
  for (const patch of [
    { title: 'Х' },
    { description: 'Х' },
    { parentId: '' },
    { pinned: true },
    { isReadme: true },
    { isPrivate: true },
    { isCategory: true },
  ]) {
    expect(noteFormsEqual(a, { ...a, ...patch })).toBe(false)
  }
})
