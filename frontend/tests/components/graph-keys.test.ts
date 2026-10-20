import { expect, test } from 'bun:test'

import { handleGraphKey } from '@/pages/graph/model/keys.ts'

function key(extra: Record<string, unknown> = {}) {
  const calls: string[] = []
  let prevented = false
  const env = {
    active: true,
    linking: false,
    selectMode: false,
    full: false,
    menuOpen: false,
    ownerless: 0,
    canClear: true,
    cancelLink: () => calls.push('cancel'),
    toggleSelect: () => calls.push('select'),
    toggleFull: () => calls.push('full'),
    save: () => calls.push('save'),
    undo: () => calls.push('undo'),
    fit: () => calls.push('fit'),
    relayout: () => calls.push('relayout'),
    clear: () => calls.push('clear'),
    ...extra,
  }
  const e = {
    key: 'a',
    code: 'KeyA',
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    target: null,
    preventDefault() {
      prevented = true
    },
  }
  return {
    calls,
    env,
    press(over: Record<string, unknown> = {}) {
      prevented = false
      handleGraphKey({ ...e, ...over }, env)
      return prevented
    },
  }
}

test('Esc снимает связывание раньше выделения и полного экрана', () => {
  const k = key({ linking: true, selectMode: true, full: true })
  k.press({ key: 'Escape', code: 'Escape' })
  expect(k.calls).toEqual(['cancel'])
  k.env.linking = false
  k.press({ key: 'Escape', code: 'Escape' })
  expect(k.calls).toEqual(['cancel', 'select'])
  k.env.selectMode = false
  k.press({ key: 'Escape', code: 'Escape' })
  expect(k.calls).toEqual(['cancel', 'select', 'full'])
})

test('Ctrl+S сохраняет даже из поля ввода, но не при карточках без владельца и не поверх диалога', () => {
  const k = key()
  const input = document.createElement('input')
  expect(k.press({ code: 'KeyS', ctrlKey: true, target: input })).toBe(true)
  expect(k.calls).toEqual(['save'])
  k.env.ownerless = 2
  k.press({ code: 'KeyS', ctrlKey: true })
  expect(k.calls).toEqual(['save'])
  const dialog = document.createElement('div')
  dialog.className = 'backdrop'
  dialog.innerHTML = '<div role="dialog"></div>'
  document.body.append(dialog)
  k.env.ownerless = 0
  k.press({ code: 'KeyS', ctrlKey: true })
  expect(k.calls).toEqual(['save'])
  dialog.remove()
})

test('в поле ввода до карты не доходят ни V, ни Ctrl+Z', () => {
  const k = key()
  const input = document.createElement('textarea')
  expect(k.press({ code: 'KeyV', target: input })).toBe(false)
  expect(k.press({ code: 'KeyZ', ctrlKey: true, target: input })).toBe(false)
  expect(k.calls).toEqual([])
  expect(k.press({ code: 'KeyZ', ctrlKey: true })).toBe(true)
  expect(k.calls).toEqual(['undo'])
})

test('V, F, R и Shift+Del доходят до карты, модификатор без своего сочетания — нет', () => {
  const k = key()
  expect(k.press({ code: 'KeyV' })).toBe(true)
  expect(k.press({ code: 'KeyF' })).toBe(true)
  expect(k.press({ code: 'KeyR' })).toBe(true)
  expect(k.press({ code: 'Delete', shiftKey: true })).toBe(true)
  expect(k.calls).toEqual(['select', 'fit', 'relayout', 'clear'])
  k.env.canClear = false
  expect(k.press({ code: 'Delete', shiftKey: true })).toBe(false)
  expect(k.press({ code: 'KeyV', ctrlKey: true })).toBe(false)
  k.env.active = false
  expect(k.press({ code: 'KeyF' })).toBe(false)
})

test('Esc сначала снимает подсветку линии, потом идёт обычная цепочка', () => {
  const k = key({ hasLit: true, clearLit: () => k.calls.push('unlit'), selectMode: true })
  k.press({ key: 'Escape', code: 'Escape' })
  expect(k.calls).toEqual(['unlit'])
  k.env.hasLit = false
  k.press({ key: 'Escape', code: 'Escape' })
  expect(k.calls).toEqual(['unlit', 'select'])
})
