// Что показать на экране карты: уровень детализации карточек по видимой площади и модель миникарты.
import { CARD, SQUARE } from './constants.ts'
import { STUB, clampK } from './geometry.ts'
import type { GraphEdgeView, GraphNodeView } from './types.ts'
import type { GraphZone } from './zones.ts'

type BoxNode = {
  type?: string
  desc?: string
  x: number
  y: number
  w: number
  h: number
  sw?: number
  sh?: number
}

function nominalW(n: BoxNode) {
  if (n.type === 'attachment' || n.type === 'cluster') return SQUARE
  if (n.type === 'quest' || n.type === 'note') return CARD[n.type].w
  if (n.type === 'step' && n.desc) return CARD.stepBody.w
  return n.w > 0 ? n.w : 160
}

function nominalH(n: BoxNode) {
  if (n.type === 'attachment' || n.type === 'cluster') return SQUARE
  if (n.type === 'quest' || n.type === 'note') return CARD[n.type].max
  if (n.type === 'step' && n.desc) return CARD.stepBody.max
  return n.h > 0 ? n.h : 36
}

/**
 * Rectangle the card is actually painted with. Position follows the measured
 * centre (`n.w` / `n.h`); width and height follow the stretched size, which
 * can be set while the measured box is still 0.
 */
export function paintedRect(n: BoxNode) {
  const w = n.sw ?? (n.w > 0 ? n.w : nominalW(n))
  const h = n.sh ?? (n.h > 0 ? n.h : nominalH(n))
  const left = n.x - n.w / 2
  const top = n.y - n.h / 2
  return { x: left + w / 2, y: top + h / 2, w, h }
}

/** Give a fresh node a real box before layout, so culling does not treat it as a point. */
export function ensureCardBox(n: BoxNode) {
  if (!(n.w > 0)) n.w = n.sw ?? nominalW(n)
  if (!(n.h > 0)) n.h = n.sh ?? nominalH(n)
}

/**
 * Copy the previous place onto a rebuilt node and fill an empty box.
 * Growing that box moves the centre so the visible corner stays put.
 */
export function placeCardBoxes(
  nodes: (BoxNode & { key: string })[],
  previous: ReadonlyMap<string, BoxNode>,
  saved: { get(key: string): { x: number; y: number } | undefined }
) {
  for (const n of nodes) {
    const old = previous.get(n.key)
    if (old) {
      n.x = old.x
      n.y = old.y
      if (old.w > 0) n.w = old.w
      if (old.h > 0) n.h = old.h
    }
    const hadW = n.w > 0
    const hadH = n.h > 0
    ensureCardBox(n)
    if (!hadW) n.x += n.w / 2
    if (!hadH) n.y += n.h / 2
    const spot = old ? saved.get(n.key) : undefined
    if (spot && old && spot.x === old.x && spot.y === old.y && (!hadW || !hadH)) {
      spot.x = n.x
      spot.y = n.y
    }
  }
}

// How much of a card is built right now, by its share of the map area on screen:
// 'lite' — frame only (< 0.5%, or well outside the window); 'title' — the head
// without the markdown body (0.5–5%); full (not in the map) — everything.
// Cards keep their box, so nothing moves; the body is simply not built until
// it matters. Each threshold has ±5% slack around the card's current tier, so
// a card sitting right on a border does not flicker while zooming.
// Letters scale with the card, so a line keeps the same number of characters
// at every zoom. Full text waits until the card covers about 5% of the window.
const LITE_AREA = 0.005
const FULL_AREA = 0.05
const SLACK = 1.05

export type Tier = 'lite' | 'title'

/** Tier of each card now (cards in no map entry are drawn in full). Returns `prev` itself when nothing changed. */
export function computeTiers({
  nodes,
  cam,
  stage,
  ready,
  editingKey,
  prev,
}: {
  nodes: GraphNodeView[] | undefined
  cam: { k: number; x: number; y: number }
  stage: { w: number; h: number }
  ready: boolean
  editingKey?: string
  prev: Map<string, string>
}): Map<string, string> {
  const next = new Map<string, string>()
  const { k, x: cx, y: cy } = cam
  if (ready && nodes && stage.w && stage.h) {
    const W = stage.w
    const H = stage.h
    for (const n of nodes) {
      if (n.type !== 'quest' && n.type !== 'note' && !(n.type === 'step' && n.desc)) continue
      if (editingKey === n.key) continue
      const box = paintedRect(n)
      const sx = (box.x - box.w / 2) * k + cx
      const sy = (box.y - box.h / 2) * k + cy
      const sw = box.w * k
      const sh = box.h * k
      // One window of margin each way, so panning does not reveal empty frames.
      const off = sx + sw < -W || sx > 2 * W || sy + sh < -H || sy > 2 * H
      if (off) {
        next.set(n.key, 'lite')
        continue
      }
      const share = (sw * sh) / (W * H)
      const was = prev.get(n.key) ?? 'full'
      const lite = share < (was === 'lite' ? LITE_AREA * SLACK : LITE_AREA / SLACK)
      const full = share >= (was === 'full' ? FULL_AREA / SLACK : FULL_AREA * SLACK)
      if (lite) next.set(n.key, 'lite')
      else if (!full) next.set(n.key, 'title')
    }
  }
  if (next.size === prev.size && [...next].every(([key, t]) => prev.get(key) === t)) return prev
  return next
}

const MINI_W = 170
const MINI_H = 120

/** Minimap: every card as a small rectangle and the visible part as a frame (null while there is nothing to show). */
export function minimapModel({
  nodes: all,
  zones = [],
  ready,
  cam,
  stage,
}: {
  nodes: GraphNodeView[]
  zones?: GraphZone[]
  ready: boolean
  cam: { k: number; x: number; y: number }
  stage: { w: number; h: number }
}) {
  const nodes = all.filter((n) => ready && Number.isFinite(n.x) && paintedRect(n).w)
  if ((!nodes.length && !zones.length) || !stage.w) return null
  const vx = -cam.x / cam.k
  const vy = -cam.y / cam.k
  const vw = stage.w / cam.k
  const vh = stage.h / cam.k
  let x0 = vx,
    y0 = vy,
    x1 = vx + vw,
    y1 = vy + vh
  for (const n of nodes) {
    const box = paintedRect(n)
    x0 = Math.min(x0, box.x - box.w / 2)
    x1 = Math.max(x1, box.x + box.w / 2)
    y0 = Math.min(y0, box.y - box.h / 2)
    y1 = Math.max(y1, box.y + box.h / 2)
  }
  for (const z of zones) {
    x0 = Math.min(x0, z.x)
    y0 = Math.min(y0, z.y)
    x1 = Math.max(x1, z.x + z.w)
    y1 = Math.max(y1, z.y + z.h)
  }
  const pad = Math.max(x1 - x0, y1 - y0) * 0.04
  x0 -= pad
  y0 -= pad
  x1 += pad
  y1 += pad
  const sc = Math.min(MINI_W / (x1 - x0), MINI_H / (y1 - y0))
  return { x0, y0, sc, w: (x1 - x0) * sc, h: (y1 - y0) * sc, nodes, zones, view: { x: vx, y: vy, w: vw, h: vh } }
}

/**
 * The camera that shows every node in a stage of the given size (centered, never zoomed in past 1:1), or null when there is
 * nothing to fit. Nodes are given by their centre and size, as the map keeps them.
 */
export function fitCamera(
  nodes: { x: number; y: number; w: number; h: number }[],
  stage: { w: number; h: number }
): { k: number; x: number; y: number } | null {
  if (!stage.w || !nodes.length) return null
  let x0 = Infinity
  let y0 = Infinity
  let x1 = -Infinity
  let y1 = -Infinity
  for (const n of nodes) {
    x0 = Math.min(x0, n.x - n.w / 2)
    x1 = Math.max(x1, n.x + n.w / 2)
    y0 = Math.min(y0, n.y - n.h / 2)
    y1 = Math.max(y1, n.y + n.h / 2)
  }
  const k = Math.min(1, (stage.w - 40) / Math.max(1, x1 - x0), (stage.h - 40) / Math.max(1, y1 - y0))
  return { k, x: stage.w / 2 - ((x0 + x1) / 2) * k, y: stage.h / 2 - ((y0 + y1) / 2) * k }
}

/** World bounds with a screen-space buffer, so cards mount before entering the window. */
export function viewportBounds(
  cam: { x: number; y: number; k: number },
  stage: { w: number; h: number },
  padding = 240
) {
  if (!stage.w || !stage.h || cam.k <= 0) return null
  const pad = padding / cam.k
  return {
    x0: -cam.x / cam.k - pad,
    y0: -cam.y / cam.k - pad,
    x1: (stage.w - cam.x) / cam.k + pad,
    y1: (stage.h - cam.y) / cam.k + pad,
  }
}

type ViewportBounds = ReturnType<typeof viewportBounds>

export function boxInViewport(n: BoxNode, bounds: ViewportBounds) {
  const box = paintedRect(n)
  return (
    !bounds ||
    (box.x + box.w / 2 >= bounds.x0 &&
      box.x - box.w / 2 <= bounds.x1 &&
      box.y + box.h / 2 >= bounds.y0 &&
      box.y - box.h / 2 <= bounds.y1)
  )
}

/** Camera that fits a world rectangle in the stage, zooming out as far as needed and never closer than 2.5. */
export function cameraOnRect(
  rect: { x: number; y: number; w: number; h: number },
  stage: { w: number; h: number },
  fromK: number
) {
  if (!stage.w || !stage.h || rect.w < 1 || rect.h < 1) return null
  const margin = 48
  const wanted = Math.min((stage.w - margin * 2) / rect.w, (stage.h - margin * 2) / rect.h, 2.5)
  const k = clampK(wanted, fromK, wanted)
  const cx = rect.x + rect.w / 2
  const cy = rect.y + rect.h / 2
  return { k, x: stage.w / 2 - cx * k, y: stage.h / 2 - cy * k }
}

/** Include crossing edges even when both cards are outside. Cubic curves stay inside their control hull. */
export function edgeInViewport(e: GraphEdgeView, bounds: ViewportBounds) {
  if (!bounds) return true
  const endX = e.ex - e.fx * STUB
  const endY = e.ey - e.fy * STUB
  const off = Math.max(30, Math.abs(e.fx ? endX - e.sx : endY - e.sy) / 2)
  const x0 = Math.min(e.sx, e.ex, e.sx + e.fx * off, endX - e.fx * off)
  const x1 = Math.max(e.sx, e.ex, e.sx + e.fx * off, endX - e.fx * off)
  const y0 = Math.min(e.sy, e.ey, e.sy + e.fy * off, endY - e.fy * off)
  const y1 = Math.max(e.sy, e.ey, e.sy + e.fy * off, endY - e.fy * off)
  // Arrowheads, highlight stroke and the menu hit area extend beyond the curve.
  const pad = 32
  return x1 + pad >= bounds.x0 && x0 - pad <= bounds.x1 && y1 + pad >= bounds.y0 && y0 - pad <= bounds.y1
}
