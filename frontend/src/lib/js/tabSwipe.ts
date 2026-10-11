// Switching header tabs with a horizontal finger swipe (quest=342).
// Rule: a swipe is a swipe only when nothing under the finger would take that
// movement itself — no element that can still scroll that way, no field, no
// surface that handles touch on its own (the map), no dialog or menu on top.

/** Clearly horizontal and long enough: 1 = finger went left (next tab), -1 = right, 0 = not a swipe. */
export function swipeDir(dx: number, dy: number, { min = 60, ratio = 1.5 } = {}) {
  if (Math.abs(dx) < min || Math.abs(dx) < Math.abs(dy) * ratio) return 0
  return dx < 0 ? 1 : -1
}

/** Which ways a horizontal scroller can still move: `next` — finger to the left reveals more. */
export function scrollRoom(el: Element) {
  const max = el.scrollWidth - el.clientWidth
  return { next: el.scrollLeft < max - 1, prev: el.scrollLeft > 1 }
}

// Starts this close to a screen edge belong to the system "back" gesture.
const EDGE = 24

/**
 * What under `target` (up to `root`) would claim a horizontal drag: null when
 * nothing does (a swipe is possible), 'own' when the area handles touch itself,
 * or the horizontal scrollers met on the way, with the room each had.
 */
function inspect(target: EventTarget | null, root: Element): 'own' | { next: boolean; prev: boolean }[] | null {
  if (!(target instanceof Element)) return 'own'
  if (target.closest('input, textarea, select, [contenteditable="true"]')) return 'own'
  const scrollers: { next: boolean; prev: boolean }[] = []
  for (let el: Element | null = target; el && el !== root.parentElement; el = el.parentElement) {
    const cs = getComputedStyle(el)
    // `none` or vertical-only panning: the element takes horizontal drags itself (the map),
    // unless it says it has nothing to move right now (`data-swipe="free"`, an empty map).
    const ta = cs.touchAction
    const free = el.getAttribute('data-swipe') === 'free'
    if (!free && (ta === 'none' || (ta.includes('pan-y') && !ta.includes('pan-x')))) return 'own'
    if ((cs.overflowX === 'auto' || cs.overflowX === 'scroll') && el.scrollWidth > el.clientWidth + 1) {
      scrollers.push(scrollRoom(el))
    }
  }
  return scrollers
}

const covered = () => !!document.querySelector('.backdrop [role="dialog"], .ctx-menu')

/**
 * Svelte action: calls `onSwipe(1 | -1)` for a free one-finger horizontal swipe
 * on `node`. Touch only; mouse and pen drags are left alone. Listeners are
 * passive — the page scrolls as usual, the decision is made when the finger lifts.
 */
export function tabSwipe(node: HTMLElement, onSwipe: ((dir: 1 | -1) => void) | null | undefined) {
  let cb = onSwipe
  let start: { x: number; y: number; scrollers: { next: boolean; prev: boolean }[] } | null = null
  const reset = (): void => {
    start = null
  }
  function down(e: TouchEvent) {
    if (e.touches.length !== 1) return reset()
    const t = e.touches[0]
    if (t.clientX < EDGE || t.clientX > innerWidth - EDGE || covered()) return reset()
    const what = inspect(e.target, node)
    if (what === 'own' || what === null) return reset()
    start = { x: t.clientX, y: t.clientY, scrollers: what }
  }
  function up(e: TouchEvent) {
    if (!start || e.touches.length) return reset()
    const t = e.changedTouches[0]
    const dir = swipeDir(t.clientX - start.x, t.clientY - start.y)
    const room = start.scrollers
    reset()
    if (!dir || covered()) return
    if (room.some((r) => (dir > 0 ? r.next : r.prev))) return
    if (String(getSelection?.() ?? '')) return
    cb?.(dir as 1 | -1)
  }
  node.addEventListener('touchstart', down, { passive: true })
  node.addEventListener('touchend', up, { passive: true })
  node.addEventListener('touchcancel', reset, { passive: true })
  return {
    update(next: ((dir: 1 | -1) => void) | null | undefined) {
      cb = next
    },
    destroy() {
      node.removeEventListener('touchstart', down)
      node.removeEventListener('touchend', up)
      node.removeEventListener('touchcancel', reset)
    },
  }
}
