import { expect, test } from 'bun:test'

import { filterMenuItems } from '../../src/shared/lib/menuSearch.ts'

const items = [
  { id: 'a', label: 'Деплой: GitOps' },
  { id: 'sep', sep: true },
  { id: 'b', label: 'Прод — шпаргалка' },
  { id: 'c', label: 'Мониторинг' },
]

test('подстрока без учёта регистра; все слова запроса должны быть в названии', () => {
  expect(filterMenuItems(items, 'прод').map((i) => i.id)).toEqual(['b'])
  expect(filterMenuItems(items, 'ДЕПЛОЙ git').map((i) => i.id)).toEqual(['a'])
  expect(filterMenuItems(items, 'деплой мониторинг')).toEqual([])
})

test('пустой запрос — всё, кроме разделителей; лимит режет результат', () => {
  expect(filterMenuItems(items, '  ').map((i) => i.id)).toEqual(['a', 'b', 'c'])
  expect(filterMenuItems(items, '', 2).map((i) => i.id)).toEqual(['a', 'b'])
})
