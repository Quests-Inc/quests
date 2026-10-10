/** Placement + dismissal for small popovers (picker list, help tip).
 *
 * Popovers are position: fixed. Inside a modal their containing block is the
 * backdrop (backdrop-filter makes it one), which itself covers the viewport,
 * so viewport coordinates from getBoundingClientRect() apply as-is — and the
 * dialog's own overflow:auto can't clip them.
 */

const EDGE = 8

/**
 * Put `pop` under `anchor` (or above it when there's no room below), kept
 * inside the viewport.
 * @param {HTMLElement} anchor
 * @param {HTMLElement} pop
 * @param {{ gap?: number, matchWidth?: boolean, align?: 'start' | 'end' }} [opts]
 * @returns {'below' | 'above'}
 */
export function placePopover(anchor, pop, { gap = 4, matchWidth = false, align = 'start' } = {}) {
  const r = anchor.getBoundingClientRect()
  if (matchWidth) pop.style.minWidth = `${Math.round(r.width)}px`
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pw = pop.offsetWidth
  const ph = pop.offsetHeight
  let left = align === 'end' ? r.right - pw : r.left
  left = Math.max(EDGE, Math.min(left, vw - pw - EDGE))
  let top = r.bottom + gap
  let placement = /** @type {'below' | 'above'} */ ('below')
  if (top + ph > vh - EDGE && r.top - gap - ph >= EDGE) {
    top = r.top - gap - ph
    placement = 'above'
  }
  pop.style.left = `${Math.round(left)}px`
  pop.style.top = `${Math.round(Math.max(EDGE, top))}px`
  return placement
}

/**
 * While a popover is open: close it on a pointer-down outside `nodes`, and
 * keep it attached to its anchor when anything scrolls or the window resizes.
 * @param {() => (Element | null | undefined)[]} nodes
 * @param {() => void} onOutside
 * @param {() => void} reposition
 * @returns {() => void} cleanup
 */
export function watchPopover(nodes, onOutside, reposition) {
  const onDown = (event) => {
    const inside = nodes().some((n) => n && event.target instanceof Node && n.contains(event.target))
    if (!inside) onOutside()
  }
  const onScroll = (event) => {
    // Scrolling the popover's own list must not re-place it.
    if (nodes().some((n) => n && event.target instanceof Node && n.contains(event.target))) return
    reposition()
  }
  window.addEventListener('pointerdown', onDown, true)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', reposition)
  return () => {
    window.removeEventListener('pointerdown', onDown, true)
    window.removeEventListener('scroll', onScroll, true)
    window.removeEventListener('resize', reposition)
  }
}
