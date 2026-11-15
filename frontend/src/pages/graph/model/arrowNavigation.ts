import { typing, dialogOpen } from '@/widgets/graph-canvas'

const directions: Record<string, [number, number]> = {
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
}
/** Held arrows move in screen pixels, independently of keyboard repeat and map zoom. */
export function createArrowNavigation(deps: {
  allowed: () => boolean
  move: (dx: number, dy: number, selected: boolean) => void
}) {
  const held = new Set<string>()
  let frame = 0,
    last = 0,
    acceleratedAt: number | null = null,
    shift = false,
    selected = false
  const stop = () => {
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    held.clear()
    acceleratedAt = null
    last = 0
  }
  function tick(now: number) {
    frame = 0
    if (!deps.allowed() || dialogOpen() || typing({ target: document.activeElement }) || !held.size) {
      stop()
      return
    }
    if (shift && acceleratedAt === null) acceleratedAt = now
    if (!shift) acceleratedAt = null
    const speed = 180 * (1 + Math.min(7, acceleratedAt === null ? 0 : (now - acceleratedAt) / 300))
    const dt = last ? Math.min(50, now - last) / 1000 : 1 / 60
    last = now
    let x = 0,
      y = 0
    for (const code of held) {
      x += directions[code][0]
      y += directions[code][1]
    }
    const length = Math.hypot(x, y) || 1
    deps.move((x / length) * speed * dt, (y / length) * speed * dt, selected)
    frame = requestAnimationFrame(tick)
  }
  return {
    down(e: KeyboardEvent) {
      shift = e.shiftKey
      selected = e.ctrlKey || e.metaKey
      if (!directions[e.code] || e.defaultPrevented || e.altKey || typing(e) || !deps.allowed()) return false
      e.preventDefault()
      held.add(e.code)
      if (!frame) frame = requestAnimationFrame(tick)
      return true
    },
    up(e: KeyboardEvent) {
      shift = e.shiftKey
      selected = e.ctrlKey || e.metaKey
      held.delete(e.code)
      if (!held.size) stop()
    },
    stop,
  }
}
