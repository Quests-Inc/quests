import { expect, test } from 'bun:test'

import { groupScripts } from '../../src/pages/scripts/model/groups.ts'

const samples = [
  { id: 'a', title: 'Первый' },
  { id: 'b', title: 'Второй' },
]
const s = (id, template) => ({ id, title: `s${id}`, template })

test('scripts are grouped by sample, in the samples order, the rest last', () => {
  const g = groupScripts([s(1, 'b'), s(2), s(3, 'a'), s(4, 'b'), s(5, 'gone')], samples)
  expect(g.map((x) => [x.title, x.items.map((i) => i.id)])).toEqual([
    ['Первый', [3]],
    ['Второй', [1, 4]],
    ['Свои скрипты', [2, 5]],
  ])
})

test('empty groups are not listed', () => {
  expect(groupScripts([], samples)).toEqual([])
  expect(groupScripts([s(1)], samples).length).toBe(1)
})
