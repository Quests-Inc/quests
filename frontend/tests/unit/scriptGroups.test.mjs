import { expect, test } from 'bun:test'

import { groupScripts } from '../../src/pages/scripts/model/groups.ts'

const purposes = [
  { id: 'step', title: 'Шаги', color: '#1' },
  { id: 'note', title: 'Заметки', color: '#2' },
]
const none = { id: '', title: 'Общие', color: '#0' }
const s = (id, purpose) => ({ id, title: `s${id}`, purpose })

test('scripts are grouped by purpose, in the purposes order, the unsorted last', () => {
  const g = groupScripts([s(1, 'note'), s(2, ''), s(3, 'step'), s(4, 'note'), s(5)], purposes, none)
  expect(g.map((x) => [x.title, x.items.map((i) => i.id)])).toEqual([
    ['Шаги', [3]],
    ['Заметки', [1, 4]],
    ['Общие', [2, 5]],
  ])
})

test('empty groups are not listed', () => {
  expect(groupScripts([], purposes, none)).toEqual([])
  expect(groupScripts([s(1, 'step')], purposes, none).length).toBe(1)
})
