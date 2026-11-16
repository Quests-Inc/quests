import { expect, test } from 'bun:test'

import { benchmarkRaster } from '@/widgets/graph-canvas/model/rasterBenchmark.ts'

test('raster benchmark samples both variants and restores the camera', async () => {
  const originalFrame = globalThis.requestAnimationFrame
  globalThis.requestAnimationFrame = (callback) => {
    queueMicrotask(() => callback(performance.now()))
    return 1
  }
  const logs: string[] = []
  const debug = console.debug
  console.debug = (...args: unknown[]) => {
    logs.push(args.map(String).join(' '))
  }
  const node = document.createElement('div')
  Object.defineProperty(node, 'clientWidth', { value: 800 })
  Object.defineProperty(node, 'clientHeight', { value: 600 })
  const card = document.createElement('div')
  card.className = 'gcard'
  card.style.transform = 'translate(10px, 20px)'
  node.append(card)
  let cam = { x: 10, y: 20, k: 1 }
  const stop = benchmarkRaster(
    node,
    () => cam,
    (next) => {
      cam = next
    }
  )
  const end = Date.now() + 2000
  while (!logs.some((line) => line.includes('RASTER_BENCH_DONE')) && Date.now() < end) {
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
  stop()
  console.debug = debug
  globalThis.requestAnimationFrame = originalFrame
  expect(logs.some((line) => line.includes('RASTER_BENCH_SAMPLE'))).toBe(true)
  expect(logs.some((line) => line.includes('RASTER_BENCH_DONE'))).toBe(true)
  expect(cam).toEqual({ x: 10, y: 20, k: 1 })
})
