// Растягивание карточек: за уголок карточка меняет ширину и высоту, левый верхний угол стоит на месте.
// Размер запоминается по ключу карточки (`setSize`), двойной клик по уголку возвращает автоматический.
import type { GraphNodeView } from '@/entities/graph'

export const MIN_CARD = { w: 120, h: 44 }
export const MAX_CARD = { w: 1200, h: 1200 }

type Drag = { n: GraphNodeView; x: number; y: number; w: number; h: number; left: number; top: number; moved: boolean }

export function createCardResize(deps: {
  getK: () => number
  /** Узлы поменяли размер или место: перерисовать рёбра. */
  touchView: () => void
  getSaved: () => Map<string, { x: number; y: number }>
  setSize: (key: string, size: { w: number; h: number } | null) => void
  /** После сброса размер снова меряют по содержимому. */
  remeasure: () => void
}) {
  let drag: Drag | null = null
  let active = $state<string | null>(null)

  const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

  function onMove(e: PointerEvent) {
    if (!drag) return
    const k = deps.getK() || 1
    const dx = (e.clientX - drag.x) / k
    const dy = (e.clientY - drag.y) / k
    if (!drag.moved && Math.abs(dx) + Math.abs(dy) < 2) return
    drag.moved = true
    const w = clamp(drag.w + dx, MIN_CARD.w, MAX_CARD.w)
    const h = clamp(drag.h + dy, MIN_CARD.h, MAX_CARD.h)
    const n = drag.n
    n.sw = w
    n.sh = h
    n.w = w
    n.h = h
    // Cards are placed by their centre: moving it by half the growth keeps the top-left corner where it was.
    n.x = drag.left + w / 2
    n.y = drag.top + h / 2
    deps.touchView()
  }

  function finish() {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', finish)
    window.removeEventListener('pointercancel', finish)
    const d = drag
    drag = null
    active = null
    if (!d?.moved) return
    deps.setSize(d.n.key, { w: Math.round(d.n.w), h: Math.round(d.n.h) })
    deps.getSaved().set(d.n.key, { x: d.n.x, y: d.n.y })
    deps.touchView()
  }

  /** Pointer down on a card's corner grip. */
  function start(e: PointerEvent, n: GraphNodeView) {
    if (e.button !== 0) return
    e.stopPropagation()
    e.preventDefault()
    drag = { n, x: e.clientX, y: e.clientY, w: n.w, h: n.h, left: n.x - n.w / 2, top: n.y - n.h / 2, moved: false }
    active = n.key
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', finish)
    window.addEventListener('pointercancel', finish)
  }

  /** Back to the size the content gives. */
  function reset(n: GraphNodeView) {
    delete n.sw
    delete n.sh
    deps.setSize(n.key, null)
    deps.remeasure()
  }

  return {
    get active() {
      return active
    },
    start,
    reset,
  }
}
