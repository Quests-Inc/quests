// Что показать на экране карты: уровень детализации карточек по видимой площади и модель миникарты.
import type { GraphNodeView } from './types.ts'

// How much of a card is built right now, by its share of the map area on screen:
// 'lite' — frame only (< 0.5%, or well outside the window); 'title' — the head
// without the markdown body (0.5–1%); full (not in the map) — everything.
// Cards keep their box, so nothing moves; the body is simply not built until
// it matters. Each threshold has ±5% slack around the card's current tier, so
// a card sitting right on a border does not flicker while zooming.
const LITE_AREA = 0.005
const FULL_AREA = 0.01
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
      const sx = (n.x - n.w / 2) * k + cx
      const sy = (n.y - n.h / 2) * k + cy
      const sw = n.w * k
      const sh = n.h * k
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
  ready,
  cam,
  stage,
}: {
  nodes: GraphNodeView[]
  ready: boolean
  cam: { k: number; x: number; y: number }
  stage: { w: number; h: number }
}) {
  const nodes = all.filter((n) => ready && Number.isFinite(n.x) && n.w)
  if (!nodes.length || !stage.w) return null
  const vx = -cam.x / cam.k
  const vy = -cam.y / cam.k
  const vw = stage.w / cam.k
  const vh = stage.h / cam.k
  let x0 = vx,
    y0 = vy,
    x1 = vx + vw,
    y1 = vy + vh
  for (const n of nodes) {
    x0 = Math.min(x0, n.x - n.w / 2)
    x1 = Math.max(x1, n.x + n.w / 2)
    y0 = Math.min(y0, n.y - n.h / 2)
    y1 = Math.max(y1, n.y + n.h / 2)
  }
  const pad = Math.max(x1 - x0, y1 - y0) * 0.04
  x0 -= pad
  y0 -= pad
  x1 += pad
  y1 += pad
  const sc = Math.min(MINI_W / (x1 - x0), MINI_H / (y1 - y0))
  return { x0, y0, sc, w: (x1 - x0) * sc, h: (y1 - y0) * sc, nodes, view: { x: vx, y: vy, w: vw, h: vh } }
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
