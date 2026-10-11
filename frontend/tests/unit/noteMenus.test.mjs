import { expect, test } from 'bun:test'

import {
  moveTargetItems,
  noteCrumbs,
  noteMenuItems,
  parentMenu,
  readmeBubbles,
} from '../../src/entities/note/model/menus.ts'

const notes = [
  { id: 1, parent_id: null, title: 'корень', sort_order: 0 },
  { id: 2, parent_id: 1, title: 'ребёнок', sort_order: 0 },
  { id: 3, parent_id: 2, title: 'внук', sort_order: 0, is_readme: true },
  { id: 4, parent_id: 1, title: 'брат', sort_order: 1 },
  { id: 5, parent_id: null, title: 'другой корень', sort_order: 1 },
]

test('noteCrumbs: от корня до родителя, пустой родитель — только корень', () => {
  expect(noteCrumbs(notes, '')).toEqual([{ id: null, title: 'Корень' }])
  expect(noteCrumbs(notes, '2').map((c) => c.title)).toEqual(['Корень', 'корень', 'ребёнок'])
  expect(noteCrumbs(notes, 'x')).toEqual([{ id: null, title: 'Корень' }])
})

test('parentMenu: корень и все заметки, кроме самой и её потомков', () => {
  expect(parentMenu(notes, null).map((i) => i.id)).toEqual(['root'])
  expect(parentMenu(notes, 2).map((i) => i.id)).toEqual(['root', '1', '4', '5'])
})

test('readmeBubbles: свёрнутый README всплывает на ближайшего видимого предка', () => {
  const closed = new Set()
  expect([...readmeBubbles(notes, (id) => closed.has(id))]).toEqual([1])
  const open = new Set([1, 2])
  expect([...readmeBubbles(notes, (id) => open.has(id))]).toEqual([])
  expect([...readmeBubbles(notes, (id) => id === 1)]).toEqual([2])
})

test('moveTargetItems: вверх, корень, соседи своего уровня и уровня родителя', () => {
  const hasChildren = (id) => notes.some((n) => n.parent_id === id)
  const items = moveTargetItems(notes[1], notes, hasChildren)
  expect(items.map((i) => i.id)).toEqual([
    'move-up',
    'move:root',
    'sep-move',
    'move:1',
    'move:5',
    'sep-move-5',
    'move:4',
  ])
  expect(items.find((i) => i.id === 'move:1').checked).toBe(true)
  // своих потомков и себя в списке нет
  expect(items.map((i) => i.id)).not.toContain('move:3')
  expect(items.map((i) => i.id)).not.toContain('move:2')
})

test('noteMenuItems: пустой без заметки; при поиске нет «Переименовать»; подписи флагов по состоянию', () => {
  expect(noteMenuItems({ noteId: null, note: null, searchActive: false, exportBusy: false, moveTargets: [] })).toEqual(
    []
  )
  const items = noteMenuItems({
    noteId: 3,
    note: notes[2],
    searchActive: true,
    exportBusy: true,
    moveTargets: [{ id: 'move:root' }],
  })
  expect(items.map((i) => i.id)).not.toContain('rename')
  expect(items[0].label).toBe('Копировать note=3')
  expect(items.find((i) => i.id === 'flags').children.find((c) => c.id === 'readme').label).toBe('Снять README')
  expect(items.find((i) => i.id === 'export').children[1].label).toBe('PDF…')
  expect(items.find((i) => i.id === 'move').children).toEqual([{ id: 'move:root' }])
})
