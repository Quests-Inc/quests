import { expect, test } from 'bun:test'

import { isListSelectAll } from '@/shared/lib/listKeys.ts'
import { createMultiSelect } from '@/shared/lib/multiSelect.svelte.ts'

import { inRoot } from './root.svelte.ts'

const press = (over: Record<string, unknown> = {}) => ({
  code: 'KeyA',
  ctrlKey: true,
  shiftKey: false,
  altKey: false,
  target: document.body,
  ...over,
})

test('Ctrl+A в списке — выделение всех; в поле ввода, диалоге и рабочей области справа — нет', () => {
  expect(isListSelectAll(press())).toBe(true)
  expect(isListSelectAll(press({ ctrlKey: false, metaKey: true }))).toBe(true)
  expect(isListSelectAll(press({ ctrlKey: false }))).toBe(false)
  expect(isListSelectAll(press({ shiftKey: true }))).toBe(false)
  expect(isListSelectAll(press({ code: 'KeyB' }))).toBe(false)
  for (const sel of ['input', 'textarea', 'div.detail', 'div.notes__page', 'div.graph__stage']) {
    const [tag, cls] = sel.split('.')
    const el = document.createElement(tag)
    if (cls) el.className = cls
    document.body.append(el)
    expect(isListSelectAll(press({ target: el }))).toBe(false)
    el.remove()
  }
})

test('selectAll: выбирает все видимые, повтор на тех же — снимает, снятое якорь сбрасывает', () => {
  let m!: ReturnType<typeof createMultiSelect>
  const stop = inRoot(() => (m = createMultiSelect({ current: () => null })))
  m.selectAll([1, 2, 3])
  expect(m.list).toEqual([1, 2, 3])
  m.selectAll([1, 2, 3])
  expect(m.size).toBe(0)
  m.click({ ctrlKey: true }, 1, [1, 2, 3])
  m.selectAll([1, 2, 3])
  expect(m.size).toBe(3)
  stop()
})
