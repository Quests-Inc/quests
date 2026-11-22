import { expect, test } from 'bun:test'

import { afterPaint, resetFrameWork } from '@/shared/lib/frameTasks.ts'

test('paint precedes content; frame work is bounded and cancelled mounts never execute', () => {
  const oldRequest = globalThis.requestAnimationFrame
  const oldCancel = globalThis.cancelAnimationFrame
  const frames = new Map<number, FrameRequestCallback>()
  let id = 0
  globalThis.requestAnimationFrame = (callback) => {
    frames.set(++id, callback)
    return id
  }
  globalThis.cancelAnimationFrame = (key) => {
    frames.delete(key)
  }
  const draw = () => {
    const batch = [...frames.values()]
    frames.clear()
    for (const callback of batch) callback(performance.now())
  }
  const log: number[] = []
  const cancels: (() => void)[] = []
  let now = 0
  const origNow = performance.now.bind(performance)
  performance.now = () => now
  try {
    resetFrameWork()
    for (let i = 0; i < 10; i++)
      cancels.push(
        afterPaint(() => {
          log.push(i)
          now += 5
        })
      )
    cancels[3]()
    draw()
    expect(log).toEqual([])
    draw()
    expect(log.length).toBeGreaterThan(0)
    expect(log.length).toBeLessThanOrEqual(2)
    for (let i = 0; i < 12 && frames.size; i++) draw()
    expect(log).toEqual([0, 1, 2, 4, 5, 6, 7, 8, 9])
  } finally {
    cancels.forEach((cancel) => cancel())
    resetFrameWork()
    performance.now = origNow
    globalThis.requestAnimationFrame = oldRequest
    globalThis.cancelAnimationFrame = oldCancel
  }
})
