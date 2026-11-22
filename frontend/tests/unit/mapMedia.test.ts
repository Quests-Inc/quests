import { expect, test } from 'bun:test'

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
