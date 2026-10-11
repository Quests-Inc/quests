import { expect, test } from 'bun:test'

import { compareNotes, isUnder, withAncestors } from '../../src/entities/note/model/tree.ts'

const notes = [
  { id: 1, parent_id: null, title: 'корень' },
  { id: 2, parent_id: 1, title: 'ребёнок' },
  { id: 3, parent_id: 2, title: 'внук' },
  { id: 4, parent_id: null, title: 'посторонняя' },
]

test('withAncestors: подходящая заметка приходит с цепочкой родителей, порядок сохраняется', () => {
  expect(withAncestors(notes, [notes[2]]).map((n) => n.id)).toEqual([1, 2, 3])
  expect(withAncestors(notes, [notes[3]]).map((n) => n.id)).toEqual([4])
})

test('isUnder: потомок ниже предка, сам себя и чужие — нет', () => {
  expect(isUnder(notes, 3, 1)).toBe(true)
  expect(isUnder(notes, 1, 3)).toBe(false)
  expect(isUnder(notes, 4, 1)).toBe(false)
  expect(isUnder(notes, 3, 3)).toBe(false)
  expect(isUnder(notes, null, 1)).toBe(false)
})

test('isUnder: цикл в данных не зависает', () => {
  const loop = [
    { id: 1, parent_id: 2 },
    { id: 2, parent_id: 1 },
  ]
  expect(isUnder(loop, 1, 9)).toBe(false)
})

test('compareNotes: с детьми, затем закреплённые, затем sort_order, затем id', () => {
  const kids = new Set([10])
  const hasKids = (n) => kids.has(n.id)
  const list = [
    { id: 1, pinned: false, sort_order: 0 },
    { id: 2, pinned: true, sort_order: 5 },
    { id: 10, pinned: false, sort_order: 9 },
    { id: 3, pinned: false, sort_order: -1 },
  ]
  expect(list.sort((a, b) => compareNotes(a, b, hasKids)).map((n) => n.id)).toEqual([10, 2, 3, 1])
})
