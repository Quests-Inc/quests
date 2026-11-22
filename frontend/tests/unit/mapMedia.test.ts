import { expect, test } from 'bun:test'

import { resetImageStarts, scheduleImage } from '@/shared/lib/imageStart.ts'
import { enqueueMedia, resetMediaQueue, setMediaPaused } from '@/shared/lib/mediaQueue.ts'

test('одновременно декодируются только две картинки, пауза не пускает следующую', () => {
  resetMediaQueue()
  let running = 0
  let max = 0
  const done: Array<() => void> = []
  for (let i = 0; i < 4; i++) {
    enqueueMedia((release) => {
      running++
      max = Math.max(max, running)
      done.push(() => {
        running--
        release()
      })
    })
  }
  expect(max).toBe(2)
  expect(done.length).toBe(2)
  setMediaPaused(true)
  done[0]()
  expect(done.length).toBe(2)
  setMediaPaused(false)
  expect(done.length).toBe(3)
  resetMediaQueue()
})

test('адрес картинки занимает бюджет кадра и не ждёт загрузки', () => {
  resetImageStarts()
  const frames: Array<FrameRequestCallback> = []
  const orig = globalThis.requestAnimationFrame
  const origNow = performance.now.bind(performance)
  let now = 0
  performance.now = () => now
  globalThis.requestAnimationFrame = (cb) => {
    frames.push(cb)
    return frames.length
  }
  const started: number[] = []
  const cancel: Array<() => void> = []
  for (let i = 0; i < 5; i++) {
    const n = i
    cancel.push(
      scheduleImage(() => {
        started.push(n)
        now += 5
      })
    )
  }
  expect(started).toEqual([])
  expect(frames).toHaveLength(1)
  frames.shift()!(0)
  expect(started).toEqual([0, 1])
  frames.shift()!(1)
  expect(started).toEqual([0, 1, 2, 3])
  cancel[4]()
  frames.shift()!(2)
  expect(started).toEqual([0, 1, 2, 3])
  performance.now = origNow
  globalThis.requestAnimationFrame = orig
  resetImageStarts()
})
