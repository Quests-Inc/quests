import { flushSync } from 'svelte'

// Bound expensive component mounts to a small frame budget. Cancellation also
// removes work for cards that left the viewport or a board that was replaced.
const tasks = new Set<() => void>()
let frame = 0
function run() {
  frame = 0
  const start = performance.now()
  let count = 0
  for (const task of tasks) {
    tasks.delete(task)
    flushSync(task)
    if (++count >= 4 || performance.now() - start >= 6) break
  }
  if (tasks.size) frame = requestAnimationFrame(run)
}
export function afterPaint(task: () => void) {
  // First give placeholders a paint; the following frame may build their content.
  let cancelled = false
  const first = requestAnimationFrame(() => {
    if (cancelled) return
    tasks.add(task)
    if (!frame) frame = requestAnimationFrame(run)
  })
  return () => {
    cancelled = true
    cancelAnimationFrame(first)
    tasks.delete(task)
    if (!tasks.size && frame) {
      cancelAnimationFrame(frame)
      frame = 0
    }
  }
}
