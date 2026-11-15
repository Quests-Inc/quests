import { expect, test } from 'bun:test'

import { createArrowNavigation } from '@/pages/graph/model/arrowNavigation.ts'

test('arrows pan, Ctrl moves selection, Shift ramps speed, release and blur stop movement', () => {
  const raf = globalThis.requestAnimationFrame,
    caf = globalThis.cancelAnimationFrame
  let callback: FrameRequestCallback | null = null
  globalThis.requestAnimationFrame = (fn) => {
    callback = fn
    return 1
  }
  globalThis.cancelAnimationFrame = () => {
    callback = null
  }
  const moves: number[][] = []
  const nav = createArrowNavigation({
    allowed: () => true,
    move: (x, y, selected) => moves.push([x, y, Number(selected)]),
  })
  const key = (type: string, code: string, ctrlKey = false, shiftKey = false) =>
    new KeyboardEvent(type, { code, key: code, ctrlKey, shiftKey, cancelable: true })
  const frame = (at: number) => {
    const fn = callback!
    callback = null
    fn(at)
  }
  try {
    const first = key('keydown', 'ArrowRight')
    expect(nav.down(first)).toBe(true)
    expect(first.defaultPrevented).toBe(true)
    frame(100)
    frame(150)
    expect(moves.at(-1)).toEqual([9, 0, 0])
    nav.down(key('keydown', 'ShiftLeft', true, true))
    frame(200)
    frame(500)
    expect(moves.at(-1)![0]).toBeGreaterThan(9)
    expect(moves.at(-1)![2]).toBe(1)
    nav.up(key('keyup', 'ArrowRight'))
    expect(callback).toBeNull()
    const input = document.createElement('input')
    input.addEventListener('keydown', (e) => expect(nav.down(e)).toBe(false))
    input.dispatchEvent(key('keydown', 'ArrowLeft'))
    nav.down(key('keydown', 'ArrowDown'))
    nav.stop()
    expect(callback).toBeNull()
  } finally {
    nav.stop()
    globalThis.requestAnimationFrame = raf
    globalThis.cancelAnimationFrame = caf
  }
})
