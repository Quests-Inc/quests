// Геометрия карты связей: на каких сторонах карточек сидит ребро, как оно рисуется и как расходятся концы рёбер.
// Чистые функции без состояния — страница карты только вызывает их.

/** Размер и центр карточки (или шара) на карте. */
export interface Box {
  x: number
  y: number
  w: number
  h: number
  key?: string
}

/** Ребро с его концами; `oa` / `ob` — смещения точек выхода и входа вдоль сторон, их заполняет `spreadEnds`. */
export interface EdgeItem<E = unknown> {
  e: E
  a: Box
  b: Box
  oa?: number
  ob?: number
}

/** Straight run at the end of an edge, so the arrowhead sits on the line. */
export const STUB = 16

export const MIN_K = 0.2
export const MAX_K = 2.5

/** Limits a zoom step that starts at `from`. Fit can go below MIN_K on a big
 * map; from there zooming in must not jump straight to MIN_K. */
export const clampK = (k: number, from: number) => Math.max(Math.min(MIN_K, from), Math.min(Math.max(MAX_K, from), k))

/** Which sides of a and b an edge uses: facing sides, horizontal if far apart in x. */
export function sidesOf(a: { x: number; w: number; y: number }, b: { x: number; w: number; y: number }) {
  const dx = b.x - a.x
  if (Math.abs(dx) > ((a.w + b.w) / 2) * 0.7) {
    const dir = dx > 0 ? 1 : -1
    return { vertical: false, dir, sa: dir > 0 ? 'R' : 'L', sb: dir > 0 ? 'L' : 'R' }
  }
  const dir = b.y > a.y ? 1 : -1
  return { vertical: true, dir, sa: dir > 0 ? 'B' : 'T', sb: dir > 0 ? 'T' : 'B' }
}

/**
 * Edge between the facing sides of two cards. `oa` / `ob` are the offsets,
 * along the side, of where it leaves a and where it arrives at b, so edges
 * sharing a side get their own points. Returns the path and the start point.
 */
export function edgePath(a: Box, b: Box, oa = 0, ob = 0) {
  const { vertical, dir } = sidesOf(a, b)
  if (!vertical) {
    const x1 = a.x + (dir * a.w) / 2
    const y1 = a.y + oa
    const x2 = b.x - (dir * b.w) / 2
    const y2 = b.y + ob
    const ex = x2 - dir * STUB
    const off = Math.max(30, Math.abs(ex - x1) / 2)
    return {
      d: `M${x1} ${y1} C${x1 + dir * off} ${y1} ${ex - dir * off} ${y2} ${ex} ${y2} L${x2} ${y2}`,
      sx: x1,
      sy: y1,
      ex: x2,
      ey: y2,
      fx: dir,
      fy: 0,
    }
  }
  const x1 = a.x + oa
  const y1 = a.y + (dir * a.h) / 2
  const x2 = b.x + ob
  const y2 = b.y - (dir * b.h) / 2
  const ey = y2 - dir * STUB
  const off = Math.max(30, Math.abs(ey - y1) / 2)
  return {
    d: `M${x1} ${y1} C${x1} ${y1 + dir * off} ${x2} ${ey - dir * off} ${x2} ${ey} L${x2} ${y2}`,
    sx: x1,
    sy: y1,
    ex: x2,
    ey: y2,
    fx: 0,
    fy: dir,
  }
}

/**
 * The arrowhead at the end of an edge as a closed path (the same triangle the SVG marker drew: 7 line widths long, tip one
 * tenth past the end), so a whole batch of edges needs no <marker> — those are slow to paint in bulk.
 */
export function arrowHead(e: { ex: number; ey: number; fx: number; fy: number }, lineWidth: number) {
  const u = (lineWidth * 7) / 10
  const tx = e.ex + e.fx * u
  const ty = e.ey + e.fy * u
  const bx = e.ex - e.fx * 9 * u
  const by = e.ey - e.fy * 9 * u
  const nx = -e.fy * 5 * u
  const ny = e.fx * 5 * u
  const r = (n: number) => Math.round(n * 100) / 100
  return `M${r(tx)} ${r(ty)}L${r(bx + nx)} ${r(by + ny)}L${r(bx - nx)} ${r(by - ny)}Z`
}

/**
 * Gives every edge end its own point on the card side: outgoing edges share
 * the upper/left half, incoming the lower/right half, each ordered by where
 * the other end is, so lines do not cross needlessly.
 */
export function spreadEnds(items: EdgeItem[]) {
  type End = { item: EdgeItem; end: 'a' | 'b'; other: number }
  const groups = new Map<string, End[]>()
  const push = (key: string, item: EdgeItem, end: 'a' | 'b', other: number) => {
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push({ item, end, other })
  }
  for (const item of items) {
    const { vertical, sa, sb } = sidesOf(item.a, item.b)
    push(`${item.a.key}|${sa}|out`, item, 'a', vertical ? item.b.x : item.b.y)
    push(`${item.b.key}|${sb}|in`, item, 'b', vertical ? item.a.x : item.a.y)
  }
  for (const [key, list] of groups) {
    list.sort((p, q) => p.other - q.other)
    const isOut = key.endsWith('|out')
    const node = list[0].end === 'a' ? list[0].item.a : list[0].item.b
    const side = key.split('|')[1]
    const len = side === 'L' || side === 'R' ? node.h : node.w
    const lo = isOut ? -0.4 * len : 0.04 * len
    const hi = isOut ? -0.04 * len : 0.4 * len
    list.forEach((x, i) => {
      const off = lo + ((hi - lo) * (i + 0.5)) / list.length
      if (x.end === 'a') x.item.oa = off
      else x.item.ob = off
    })
  }
}
