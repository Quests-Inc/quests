import { expect, test } from 'bun:test'

import { handleCanvasKey, type CanvasKeyEnv } from '@/widgets/graph-canvas'

function rig(extra: Partial<CanvasKeyEnv> = {}) {
  const calls: string[] = []
  const env: CanvasKeyEnv = {
    linking: false,
    selectMode: false,
    full: false,
    cancelLink: () => calls.push('cancel'),
    toggleSelect: () => calls.push('select'),
    toggleFull: () => calls.push('full'),
    fit: () => calls.push('fit'),
    zoom: (d) => calls.push(`zoom${d}`),
    undo: () => calls.push('undo'),
    redo: () => calls.push('redo'),
    selectAll: () => calls.push('all'),
    remove: () => calls.push('remove'),
    ...extra,
  }
  let prevented = false
  const press = (code: string, over: Record<string, unknown> = {}) => {
    prevented = false
    const handled = handleCanvasKey(
      {
        key: code,
        code,
        altKey: false,
        ctrlKey: false,
        metaKey: false,
        shiftKey: false,
        target: null,
        preventDefault: () => (prevented = true),
        ...over,
      },
      env
    )
    return { handled, prevented }
  }
  return { calls, env, press }
}

test('V, F, зум и Delete работают по физической клавише', () => {
  const k = rig()
  for (const c of ['KeyV', 'KeyF', 'Equal', 'NumpadAdd', 'Minus', 'Digit0', 'Delete', 'Backspace']) k.press(c)
  expect(k.calls).toEqual(['select', 'fit', 'zoom1', 'zoom1', 'zoom-1', 'zoom0', 'remove', 'remove'])
})

test('Ctrl+Z отменяет, Ctrl+Shift+Z и Ctrl+Y повторяют, Ctrl+A выделяет всё', () => {
  const k = rig()
  k.press('KeyZ', { ctrlKey: true })
  k.press('KeyZ', { ctrlKey: true, shiftKey: true })
  k.press('KeyY', { metaKey: true })
  k.press('KeyA', { ctrlKey: true })
  expect(k.calls).toEqual(['undo', 'redo', 'redo', 'all'])
})

test('чего страница не передала — той клавиши нет и событие не гасится', () => {
  const k = rig({ redo: undefined, selectAll: undefined })
  expect(k.press('KeyY', { ctrlKey: true })).toEqual({ handled: false, prevented: false })
  expect(k.press('KeyA', { ctrlKey: true })).toEqual({ handled: false, prevented: false })
  expect(k.calls).toEqual([])
})

test('Esc по очереди: связь, режим выделения, полный экран', () => {
  const k = rig({ linking: true, selectMode: true, full: true })
  k.press('Escape')
  k.env.linking = false
  k.press('Escape')
  k.env.selectMode = false
  k.press('Escape')
  expect(k.press('Escape').handled).toBe(true)
  k.env.full = false
  expect(k.press('Escape').handled).toBe(false)
  expect(k.calls.slice(0, 3)).toEqual(['cancel', 'select', 'full'])
})

test('в поле ввода и с Alt клавиши холста не срабатывают', () => {
  const k = rig()
  const input = document.createElement('input')
  expect(k.press('KeyV', { target: input }).handled).toBe(false)
  expect(k.press('Delete', { target: input }).handled).toBe(false)
  expect(k.press('KeyF', { altKey: true }).handled).toBe(false)
  expect(k.calls).toEqual([])
})
