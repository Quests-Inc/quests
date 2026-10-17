import { expect, test } from 'bun:test'

import { createMultiSelect, pickRange } from '@/pages/journal/model/multiSelect.svelte.ts'

import { inRoot } from './root.svelte.ts'

const order = [5, 6, 7, 8, 9]

function make(current: number | null = null) {
  let cur = current
  let ms!: ReturnType<typeof createMultiSelect>
  inRoot(() => {
    ms = createMultiSelect({ current: () => cur })
  })
  return { ms, open: (id: number | null) => (cur = id) }
}

test('pickRange: диапазон в любую сторону, нет в списке — только цель', () => {
  expect(pickRange(order, 6, 8)).toEqual([6, 7, 8])
  expect(pickRange(order, 8, 6)).toEqual([6, 7, 8])
  expect(pickRange(order, 99, 7)).toEqual([7])
})

test('Ctrl-клик добавляет и убирает; первый Ctrl-клик берёт с собой открытый квест', () => {
  const { ms } = make(5)
  expect(ms.click({ ctrlKey: true }, 7, order)).toBe(true)
  expect(ms.list.sort()).toEqual([5, 7])
  ms.click({ metaKey: true }, 7, order)
  expect(ms.list).toEqual([5])
  ms.click({ ctrlKey: true }, 5, order)
  expect(ms.size).toBe(0)
})

test('Shift-клик: диапазон от якоря, повторный Shift пересчитывает от того же якоря', () => {
  const { ms } = make(null)
  expect(ms.click({}, 6, order)).toBe(false) // обычный клик задаёт якорь
  ms.click({ shiftKey: true }, 8, order)
  expect(ms.list.sort()).toEqual([6, 7, 8])
  ms.click({ shiftKey: true }, 5, order)
  expect(ms.list.sort()).toEqual([5, 6])
})

test('Ctrl+Shift добавляет диапазон к выбранному', () => {
  const { ms } = make(null)
  ms.click({}, 5, order)
  ms.click({ ctrlKey: true }, 5, order)
  ms.click({ ctrlKey: true }, 5, order)
  ms.click({ ctrlKey: true, shiftKey: true }, 7, order)
  expect(ms.list.sort()).toEqual([5, 6, 7])
  ms.click({ ctrlKey: true }, 9, order)
  ms.click({ ctrlKey: true, shiftKey: true }, 9, order)
  expect(ms.has(9)).toBe(true)
})

test('обычный клик сбрасывает выделение и отдаёт строку вызывающему', () => {
  const { ms } = make(null)
  ms.click({ ctrlKey: true }, 6, order)
  ms.click({ ctrlKey: true }, 7, order)
  expect(ms.click({}, 9, order)).toBe(false)
  expect(ms.size).toBe(0)
})

test('targets: группа, если квест в ней, иначе он один; prune убирает исчезнувшие', () => {
  const { ms } = make(null)
  ms.click({ ctrlKey: true }, 6, order)
  ms.click({ ctrlKey: true }, 7, order)
  expect(ms.targets(7).sort()).toEqual([6, 7])
  expect(ms.targets(9)).toEqual([9])
  ms.prune((id) => id !== 6)
  expect(ms.list).toEqual([7])
  expect(ms.targets(7)).toEqual([7])
})
