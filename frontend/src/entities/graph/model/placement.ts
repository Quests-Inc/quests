// Расстановка на карте: возврат рук поставленных карточек и сдвиг блока вложений на свободное место.
import type { GraphGroupView, GraphNodeView } from './types.ts'

/** Puts hand-placed cards back, and an expanded block where its cluster was. */
export function restoreHandPlaced(
  nodes: GraphNodeView[],
  saved: Map<string, { x: number; y: number }>,
  groups: GraphGroupView[]
) {
  const handPlaced = new Set()
  for (const n of nodes) {
    const p = saved.get(n.key)
    if (p) {
      n.x = p.x
      n.y = p.y
      handPlaced.add(n.key)
    }
  }
  for (const g of groups) {
    const spot = g.type === 'attachment' ? saved.get(`cluster:${g.owner}`) : null
    const members = nodes.filter((n) => n.group === g.key)
    if (!spot || !members.length || members.some((m) => handPlaced.has(m.key))) continue
    const cx = (Math.min(...members.map((m) => m.x - m.w / 2)) + Math.max(...members.map((m) => m.x + m.w / 2))) / 2
    const cy = (Math.min(...members.map((m) => m.y - m.h / 2)) + Math.max(...members.map((m) => m.y + m.h / 2))) / 2
    const dx = spot.x - cx
    const dy = spot.y - cy
    for (const m of members) {
      m.x += dx
      m.y += dy
    }
    // The spot may be occupied; slide the block to the nearest free place.
    const shift = freeShift(nodes, g, members, groups)
    for (const m of members) {
      m.x += shift.dx
      m.y += shift.dy
    }
  }
}

const FRAME = { left: 14, right: 14, top: 44, bottom: 14 }
const frameOf = (ms: GraphNodeView[]) => ({
  x0: Math.min(...ms.map((m) => m.x - m.w / 2)) - FRAME.left,
  x1: Math.max(...ms.map((m) => m.x + m.w / 2)) + FRAME.right,
  y0: Math.min(...ms.map((m) => m.y - m.h / 2)) - FRAME.top,
  y1: Math.max(...ms.map((m) => m.y + m.h / 2)) + FRAME.bottom,
})

/** Smallest nudge (down first, then sideways/up) that puts a block's frame
 * clear of every other card and group frame. {0,0} if it is already clear
 * or nothing nearby is free. */
export function freeShift(
  nodes: GraphNodeView[],
  g: { key: string },
  members: GraphNodeView[],
  allGroups: { key: string }[]
) {
  const mine = new Set(members.map((m: { key: any }) => m.key))
  const gap = 18
  const obstacles: { x0: number; x1: any; y0: number; y1: any }[] = []
  for (const n of nodes) {
    if (mine.has(n.key)) continue
    obstacles.push({ x0: n.x - n.w / 2, x1: n.x + n.w / 2, y0: n.y - n.h / 2, y1: n.y + n.h / 2 })
  }
  for (const og of allGroups) {
    if (og.key === g.key) continue
    const ms = nodes.filter((n) => n.group === og.key)
    if (ms.length) obstacles.push(frameOf(ms))
  }
  const f = frameOf(members)
  const clear = (dx: number, dy: number) =>
    !obstacles.some(
      (o) => f.x0 + dx < o.x1 + gap && f.x1 + dx > o.x0 - gap && f.y0 + dy < o.y1 + gap && f.y1 + dy > o.y0 - gap
    )
  if (clear(0, 0)) return { dx: 0, dy: 0 }
  // Directions in order of preference: down, right, up, left, then diagonals.
  const dirs = [
    [0, 1],
    [1, 0],
    [0, -1],
    [-1, 0],
    [1, 1],
    [-1, 1],
    [1, -1],
    [-1, -1],
  ]
  for (let r = 40; r <= 3000; r += 40) {
    for (const [ux, uy] of dirs) {
      if (clear(ux * r, uy * r)) return { dx: ux * r, dy: uy * r }
    }
  }
  return { dx: 0, dy: 0 }
}
