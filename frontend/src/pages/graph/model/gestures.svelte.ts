// Жесты на поле карты: панорама и зум колесом, щипок двумя пальцами, перетаскивание карточек, блоков и выделенного,
// выделение рамкой. Контроллер держит камеру и текущий жест; всё остальное (выбор, связывание, позиции) — через deps.
import { clampK, type GraphNodeView } from '@/entities/graph'

type Drag = {
  moved?: boolean
  kind: 'pan' | 'pinch' | 'marquee' | 'node' | 'multi' | 'group'
  n?: any
  nx?: number
  ny?: number
  items?: { m: any; x: number; y: number }[]
  members?: { m: any; x: number; y: number }[]
  k?: number
  d?: number
  wx?: number
  wy?: number
  x?: number
  y?: number
  cx?: number
  cy?: number
  additive?: boolean
  key?: string
}

export function createGraphGestures(deps: {
  getStage: () => HTMLElement | null
  nodes: () => GraphNodeView[]
  nodeMap: () => Map<string, GraphNodeView>
  /** Узлы поменяли координаты на месте: перерисовать. */
  touchView: () => void
  isLinking: () => boolean
  finishLink: (n: GraphNodeView) => unknown
  setLinkCursor: (p: { x: number; y: number }) => void
  selectMode: () => boolean
  getPicked: () => Set<string>
  setPicked: (s: Set<string>) => void
  getSaved: () => Map<string, { x: number; y: number }>
  select: (n: GraphNodeView) => void
}) {
  /** Camera: pan (x, y) and zoom (k) of the world. */
  let cam = $state({ x: 0, y: 0, k: 1 })
  let marquee = $state<{ x0: number; y0: number; x1: number; y1: number } | null>(null)
  let drag: Drag | null = null

  /** Screen point → world coordinates (where a new thing should land). */
  function worldAt(e: { clientX: number; clientY: number }) {
    const r = deps.getStage()?.getBoundingClientRect()
    if (!r) return { x: 0, y: 0 }
    return { x: (e.clientX - r.left - cam.x) / cam.k, y: (e.clientY - r.top - cam.y) / cam.k }
  }

  // Pan / zoom / card drag. Dragging a card works from anywhere except its
  // scrollable text and links, so reading and scrolling are not hijacked.
  /** Zooms to `k` keeping the stage point (px, py) on the same world point. */
  function zoomAt(k: number, px: number, py: number) {
    k = clampK(k, cam.k)
    cam = { k, x: px - ((px - cam.x) / cam.k) * k, y: py - ((py - cam.y) / cam.k) * k }
  }

  // Touch pointers currently down on the stage; two of them make a pinch.
  const touches = new Map()
  /** Puts cards moved by an interrupted drag back where they started. */
  function undoDrag() {
    if (!drag?.moved) return
    if (drag.kind === 'node') {
      drag.n.x = drag.nx
      drag.n.y = drag.ny
    }
    for (const s of drag.items ?? drag.members ?? []) {
      s.m.x = s.x
      s.m.y = s.y
    }
    deps.touchView()
  }
  function pinchStart() {
    const [a, b] = [...touches.values()]
    const r = deps.getStage()?.getBoundingClientRect()
    if (!r) return
    drag = {
      kind: 'pinch',
      d: Math.hypot(a.x - b.x, a.y - b.y) || 1,
      // World point under the midpoint: it stays under the fingers' midpoint.
      wx: ((a.x + b.x) / 2 - r.left - cam.x) / cam.k,
      wy: ((a.y + b.y) / 2 - r.top - cam.y) / cam.k,
      k: cam.k,
    }
  }
  /** Capture phase, so a second finger on a card starts a pinch, not a card drag. */
  function onTouchDown(e: {
    pointerType: string
    pointerId: number
    clientX: any
    clientY: any
    stopPropagation: () => void
    preventDefault: () => void
  }) {
    if (e.pointerType !== 'touch') return
    touches.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (touches.size !== 2) return
    e.stopPropagation()
    e.preventDefault()
    undoDrag()
    marquee = null
    pinchStart()
    deps.getStage()?.setPointerCapture(e.pointerId)
  }
  function onBgDown(e: PointerEvent) {
    if ((e.target as Element).closest('[data-gkey]')) return
    // Middle button always pans, also in selection mode.
    if (e.button === 1) {
      e.preventDefault()
      drag = { kind: 'pan', x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y }
      deps.getStage()?.setPointerCapture(e.pointerId)
      return
    }
    if (e.button !== 0) return
    if (deps.selectMode()) {
      const r = deps.getStage()?.getBoundingClientRect()
      if (!r) return
      const x = e.clientX - r.left
      const y = e.clientY - r.top
      drag = { kind: 'marquee', additive: e.shiftKey || e.ctrlKey || e.metaKey }
      marquee = { x0: x, y0: y, x1: x, y1: y }
      deps.getStage()?.setPointerCapture(e.pointerId)
      return
    }
    drag = { kind: 'pan', x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y }
    deps.getStage()?.setPointerCapture(e.pointerId)
  }
  function onCardDown(e: PointerEvent, n: GraphNodeView) {
    if (deps.isLinking()) {
      if (e.button === 0) {
        e.stopPropagation()
        void deps.finishLink(n)
      }
      return
    }
    if (deps.selectMode()) {
      if (e.button !== 0 || (e.target as Element).closest('textarea, input, .gcard__editbar, .gcard__changed')) return
      e.stopPropagation()
      const group = deps.getPicked().has(n.key)
        ? [...deps.getPicked()].map((k) => deps.nodeMap().get(k)).filter(Boolean)
        : [n]
      drag = {
        kind: 'multi',
        key: n.key,
        x: e.clientX,
        y: e.clientY,
        moved: false,
        items: group.map((m) => ({ m, x: m.x, y: m.y })),
      }
      deps.getStage()?.setPointerCapture(e.pointerId)
      return
    }
    if (e.button !== 0) return
    if (
      (e.target as Element).closest(
        'textarea, input, .gcard__editbar, .gcard__body a, .gcard__body button, .gcard__body pre'
      )
    )
      return
    const body = (e.target as Element).closest('.gcard__body')
    // Inside the text area: let selection/scrolling happen; only the frame drags.
    if (body && body.scrollHeight > body.clientHeight + 1 && e.offsetX > body.clientWidth - 14) return
    e.stopPropagation()
    drag = { kind: 'node', n, x: e.clientX, y: e.clientY, nx: n.x, ny: n.y, moved: false }
    deps.getStage()?.setPointerCapture(e.pointerId)
  }
  /** Dragging an expanded block by its frame moves all its cards together. */
  function onGroupDown(
    e: { button: number; stopPropagation: () => void; clientX: any; clientY: any; pointerId: number },
    g: { key: any }
  ) {
    if (e.button !== 0 || deps.isLinking() || deps.selectMode()) return
    e.stopPropagation()
    const members = deps.nodes().filter((n) => n.group === g.key)
    drag = {
      kind: 'group',
      members: members.map((m) => ({ m, x: m.x, y: m.y })),
      x: e.clientX,
      y: e.clientY,
      moved: false,
    }
    deps.getStage()?.setPointerCapture(e.pointerId)
  }
  function onMove(e: {
    pointerId?: any
    clientX: any
    clientY: any
    preventDefault?: (() => void) | (() => void) | (() => void)
    stopPropagation?: (() => void) | (() => void) | (() => void)
  }) {
    if (touches.has(e.pointerId)) touches.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (drag?.kind === 'pinch') {
      if (touches.size < 2) return
      const [a, b] = [...touches.values()]
      const r = deps.getStage()?.getBoundingClientRect()
      if (!r) return
      const k = clampK((drag.k * Math.hypot(a.x - b.x, a.y - b.y)) / drag.d, drag.k)
      cam = {
        k,
        x: (a.x + b.x) / 2 - r.left - drag.wx * k,
        y: (a.y + b.y) / 2 - r.top - drag.wy * k,
      }
      return
    }
    if (deps.isLinking()) deps.setLinkCursor(worldAt(e))
    if (!drag) return
    if (drag.kind === 'marquee') {
      const r = deps.getStage()?.getBoundingClientRect()
      if (r && marquee) marquee = { ...marquee, x1: e.clientX - r.left, y1: e.clientY - r.top }
      return
    }
    const dx = e.clientX - drag.x
    const dy = e.clientY - drag.y
    if (drag.kind === 'multi') {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      for (const s of drag.items) {
        s.m.x = s.x + dx / cam.k
        s.m.y = s.y + dy / cam.k
      }
      deps.touchView()
      return
    }
    if (drag.kind === 'pan') {
      cam = { ...cam, x: drag.cx + dx, y: drag.cy + dy }
    } else if (drag.kind === 'group') {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      for (const s of drag.members) {
        s.m.x = s.x + dx / cam.k
        s.m.y = s.y + dy / cam.k
      }
      deps.touchView()
    } else {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      drag.n.x = drag.nx + dx / cam.k
      drag.n.y = drag.ny + dy / cam.k
      deps.touchView()
    }
  }
  function onUp(e: { pointerId: any }) {
    touches.delete(e.pointerId)
    if (drag?.kind === 'pinch') {
      // Lifting one finger keeps panning with the other one.
      const rest = [...touches.values()][0]
      drag = rest ? { kind: 'pan', x: rest.x, y: rest.y, cx: cam.x, cy: cam.y } : null
      return
    }
    if (drag?.kind === 'marquee' && marquee) {
      const m = marquee
      marquee = null
      const w = Math.abs(m.x1 - m.x0)
      const h = Math.abs(m.y1 - m.y0)
      if (w < 4 && h < 4) {
        if (!drag.additive) deps.setPicked(new Set())
      } else {
        const x0 = (Math.min(m.x0, m.x1) - cam.x) / cam.k
        const x1 = (Math.max(m.x0, m.x1) - cam.x) / cam.k
        const y0 = (Math.min(m.y0, m.y1) - cam.y) / cam.k
        const y1 = (Math.max(m.y0, m.y1) - cam.y) / cam.k
        const next: Set<string> = drag.additive ? new Set(deps.getPicked()) : new Set()
        for (const n of deps.nodes()) {
          if (n.x + n.w / 2 >= x0 && n.x - n.w / 2 <= x1 && n.y + n.h / 2 >= y0 && n.y - n.h / 2 <= y1) next.add(n.key)
        }
        deps.setPicked(next)
      }
    }
    if (drag?.kind === 'multi') {
      if (drag.moved) {
        for (const s of drag.items) deps.getSaved().set(s.m.key, { x: s.m.x, y: s.m.y })
      } else {
        const next = new Set(deps.getPicked())
        if (next.has(drag.key)) next.delete(drag.key)
        else next.add(drag.key)
        deps.setPicked(next)
      }
    }
    if (drag?.kind === 'group' && drag.moved) {
      for (const s of drag.members) deps.getSaved().set(s.m.key, { x: s.m.x, y: s.m.y })
    }
    if (drag?.kind === 'node' && drag.moved) deps.getSaved().set(drag.n.key, { x: drag.n.x, y: drag.n.y })
    if (drag?.kind === 'node' && !drag.moved) deps.select(drag.n)
    drag = null
  }
  function onWheel(e: WheelEvent) {
    // Over a card's scrollable text the wheel scrolls the text, not the map.
    const body = e.target instanceof Element ? (e.target as Element).closest('.gcard__body, textarea') : null
    if (body && body.scrollHeight > body.clientHeight + 1) return
    e.preventDefault()
    const r = deps.getStage()?.getBoundingClientRect()
    if (!r) return
    // A touchpad pinch arrives as ctrl+wheel with small, frequent deltas:
    // follow them proportionally instead of a fixed step per event (capped,
    // so a ctrl+mouse-wheel notch is no bigger than a plain one).
    const f = e.ctrlKey ? Math.exp(-Math.max(-11, Math.min(11, e.deltaY)) * 0.01) : e.deltaY < 0 ? 1.12 : 1 / 1.12
    zoomAt(cam.k * f, e.clientX - r.left, e.clientY - r.top)
  }

  return {
    get cam() {
      return cam
    },
    set cam(v: { x: number; y: number; k: number }) {
      cam = v
    },
    get marquee() {
      return marquee
    },
    set marquee(v: { x0: number; y0: number; x1: number; y1: number } | null) {
      marquee = v
    },
    worldAt,
    zoomAt,
    onTouchDown,
    onBgDown,
    onCardDown,
    onGroupDown,
    onMove,
    onUp,
    onWheel,
  }
}
