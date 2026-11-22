// Bound expensive component mounts to a small frame budget. Cancellation also
// removes work for cards that left the viewport or a board that was replaced.
// The task only flips state. Flushing it synchronously forced a layout per card
// before the budget could notice, so a zone of notes turned into a long task
// apiece. Two state updates a frame land in one commit, and the browser lays
// that pair out once, after the script.
const tasks = new Set<() => void>()
let frame = 0
function run() {
  frame = 0
  let count = 0
  for (const task of tasks) {
    tasks.delete(task)
    task()
    if (++count >= 2) break
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
