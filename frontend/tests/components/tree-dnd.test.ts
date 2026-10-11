import { expect, test } from 'bun:test'

import { createTreeDnd } from '@/pages/notes/model/treeDnd.svelte.ts'

const notes = [
  { id: 1, parent_id: null },
  { id: 2, parent_id: 1 },
  { id: 3, parent_id: 2 },
  { id: 4, parent_id: null },
]

function event(extra: Record<string, unknown> = {}) {
  const log: string[] = []
  const dataTransfer = {
    effectAllowed: '',
    dropEffect: '',
    data: '',
    setData(_t: string, v: string) {
      this.data = v
    },
  }
  const e = {
    dataTransfer,
    preventDefault: () => log.push('prevent'),
    stopPropagation: () => log.push('stop'),
    ...extra,
  } as unknown as DragEvent
  return { e, log, dataTransfer }
}

function make(renaming = false) {
  const moves: Array<[number, number | null]> = []
  const dnd = createTreeDnd({ notes: () => notes, renaming: () => renaming, move: (id, p) => moves.push([id, p]) })
  return { dnd, moves }
}

test('перетаскивание: строка над другой подсвечивается, бросок переносит', () => {
  const { dnd, moves } = make()
  const start = event()
  dnd.onRowDragStart(start.e, 4)
  expect(start.dataTransfer.data).toBe('4')
  const over = event()
  dnd.onRowDragOver(over.e, 1)
  expect(dnd.dragOverKey).toBe('1')
  expect(over.dataTransfer.dropEffect).toBe('move')
  dnd.onRowDrop(event().e, 1)
  expect(moves).toEqual([[4, 1]])
  expect(dnd.dragOverKey).toBeNull()
})

test('нельзя бросить заметку на себя и на своего потомка', () => {
  const { dnd, moves } = make()
  dnd.onRowDragStart(event().e, 1)
  expect(dnd.canDropOn(1)).toBe(false)
  expect(dnd.canDropOn(3)).toBe(false)
  const over = event()
  dnd.onRowDragOver(over.e, 3)
  expect(dnd.dragOverKey).toBeNull()
  expect(over.log).toEqual([])
  dnd.onRowDrop(event().e, 3)
  expect(moves).toEqual([])
})

test('пустой фон списка — перенос в корень; уход курсора снимает подсветку', () => {
  const { dnd, moves } = make()
  dnd.onRowDragStart(event().e, 3)
  dnd.onListDragOver(event().e)
  expect(dnd.dragOverKey).toBe('root')
  dnd.onListDragLeave()
  expect(dnd.dragOverKey).toBeNull()
  dnd.onListDrop(event().e)
  expect(moves).toEqual([[3, null]])
})

test('во время переименования тащить нельзя', () => {
  const { dnd, moves } = make(true)
  const start = event()
  dnd.onRowDragStart(start.e, 2)
  expect(start.log).toEqual(['prevent'])
  dnd.onListDrop(event().e)
  expect(moves).toEqual([])
})
