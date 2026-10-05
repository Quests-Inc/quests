// Small force-directed layout for the relations map. Plain JS on purpose: the
// graph is capped at a couple of hundred nodes, so a library isn't worth its weight.

/** Ideal edge length by kind: ownership hugs, references stand off. */
const LEN = { structure: 110, dependency: 150, reference: 210 }

/**
 * Lays out nodes in place (sets x/y). Root sits at the origin.
 * @param {{ key: string, depth: number, x?: number, y?: number }[]} nodes
 * @param {{ from: string, to: string, kind: string }[]} edges
 * @param {{ iterations?: number }} [opts]
 */
export function layoutGraph(nodes, edges, { iterations = 320, sizeOf = () => ({ w: 120, h: 26 }) } = {}) {
  const n = nodes.length
  if (!n) return
  const byKey = new Map(nodes.map((nd) => [nd.key, nd]))
  // Seed: rings by depth, evenly spread, so results are stable between runs.
  const ring = new Map()
  for (const nd of nodes) {
    const d = nd.depth ?? 0
    ring.set(d, (ring.get(d) ?? 0) + 1)
  }
  const placed = new Map()
  for (const nd of nodes) {
    const d = nd.depth ?? 0
    const i = placed.get(d) ?? 0
    placed.set(d, i + 1)
    if (d === 0) {
      nd.x = 0
      nd.y = 0
    } else {
      const a = (i / ring.get(d)) * Math.PI * 2 + d * 0.7
      nd.x = Math.cos(a) * d * 140
      nd.y = Math.sin(a) * d * 140
    }
  }
  const links = edges
    .map((e) => ({ a: byKey.get(e.from), b: byKey.get(e.to), len: LEN[e.kind] ?? 100 }))
    .filter((l) => l.a && l.b)

  const vx = new Float64Array(n)
  const vy = new Float64Array(n)
  for (let it = 0; it < iterations; it++) {
    const t = 1 - it / iterations // cooling
    vx.fill(0)
    vy.fill(0)
    for (let i = 0; i < n; i++) {
      const a = nodes[i]
      for (let j = i + 1; j < n; j++) {
        const b = nodes[j]
        let dx = (a.x ?? 0) - (b.x ?? 0)
        let dy = (a.y ?? 0) - (b.y ?? 0)
        let d2 = dx * dx + dy * dy
        if (d2 < 1) {
          dx = (i % 2 ? 1 : -1) * 0.5
          dy = (j % 2 ? 1 : -1) * 0.5
          d2 = 1
        }
        const d = Math.sqrt(d2)
        const f = 26000 / d2
        vx[i] += (dx / d) * f
        vy[i] += (dy / d) * f
        vx[j] -= (dx / d) * f
        vy[j] -= (dy / d) * f
      }
    }
    const idx = new Map(nodes.map((nd, i) => [nd, i]))
    for (const l of links) {
      const i = idx.get(l.a)
      const j = idx.get(l.b)
      const dx = (l.b.x ?? 0) - (l.a.x ?? 0)
      const dy = (l.b.y ?? 0) - (l.a.y ?? 0)
      const d = Math.sqrt(dx * dx + dy * dy) || 1
      const f = (d - l.len) * 0.04
      vx[i] += (dx / d) * f
      vy[i] += (dy / d) * f
      vx[j] -= (dx / d) * f
      vy[j] -= (dy / d) * f
    }
    for (let i = 0; i < n; i++) {
      const nd = nodes[i]
      vx[i] -= (nd.x ?? 0) * 0.01 // gravity toward the root
      vy[i] -= (nd.y ?? 0) * 0.01
      const step = 12 * t + 0.5
      const m = Math.hypot(vx[i], vy[i]) || 1
      const cap = Math.min(m, step)
      if (nd.depth !== 0) {
        nd.x = (nd.x ?? 0) + (vx[i] / m) * cap
        nd.y = (nd.y ?? 0) + (vy[i] / m) * cap
      }
    }
  }

  // Final pass: push overlapping node boxes apart along the shallower axis.
  const gap = 10
  for (let pass = 0; pass < 60; pass++) {
    let moved = false
    for (let i = 0; i < n; i++) {
      const a = nodes[i]
      const sa = sizeOf(a)
      for (let j = i + 1; j < n; j++) {
        const b = nodes[j]
        const sb = sizeOf(b)
        const dx = (b.x ?? 0) - (a.x ?? 0)
        const dy = (b.y ?? 0) - (a.y ?? 0)
        const ox = (sa.w + sb.w) / 2 + gap - Math.abs(dx)
        const oy = (sa.h + sb.h) / 2 + gap - Math.abs(dy)
        if (ox <= 0 || oy <= 0) continue
        moved = true
        const aFixed = a.depth === 0
        const bFixed = b.depth === 0
        const share = aFixed || bFixed ? 1 : 0.5
        if (ox < oy) {
          const push = ox * share * (dx >= 0 ? 1 : -1)
          if (!aFixed) a.x = (a.x ?? 0) - push
          if (!bFixed) b.x = (b.x ?? 0) + push
        } else {
          const push = oy * share * (dy >= 0 ? 1 : -1)
          if (!aFixed) a.y = (a.y ?? 0) - push
          if (!bFixed) b.y = (b.y ?? 0) + push
        }
      }
    }
    if (!moved) break
  }
}

const TYPE_ORDER = { step: 0, cluster: 1, attachment: 1, note: 2, quest: 3, template: 4, questline: 5 }
const EDGE_RANK = { structure: 0, dependency: 1, reference: 2 }
/** Leaf siblings of these types, from this count up, are packed into a block.
 * Steps are not: a quest rarely has more than ten and they read better in a column. */
const BALL_TYPES = new Set(['attachment'])
const BALL_MIN = 6

/**
 * Packs boxes into a compact block: rows wrapped to a wide-ish rectangle,
 * reading order kept (left to right, top to bottom), with a header strip on
 * top for the group's label. Offsets are from the block centre.
 * @param {{ w: number, h: number }[]} items
 */
export function packBall(items, gap = 12, pad = 14, head = 30) {
  const area = items.reduce((a, b) => a + (b.w + gap) * (b.h + gap), 0)
  const maxW = Math.max(...items.map((i) => i.w))
  const target = Math.max(maxW, Math.sqrt(area * 1.6))
  const rows = []
  let row = []
  let rowW = 0
  for (const it of items) {
    const add = (row.length ? gap : 0) + it.w
    if (row.length && rowW + add > target) {
      rows.push(row)
      row = []
      rowW = 0
    }
    rowW += (row.length ? gap : 0) + it.w
    row.push(it)
  }
  if (row.length) rows.push(row)
  const widths = rows.map((r) => r.reduce((a, b) => a + b.w, 0) + (r.length - 1) * gap)
  const heights = rows.map((r) => Math.max(...r.map((i) => i.h)))
  const w = Math.max(...widths) + pad * 2
  const h = heights.reduce((a, b) => a + b, 0) + (rows.length - 1) * gap + pad * 2 + head
  const pos = []
  let y = -h / 2 + pad + head
  rows.forEach((r, i) => {
    let x = -w / 2 + pad
    for (const it of r) {
      pos.push({ item: it, dx: x + it.w / 2, dy: y + heights[i] / 2 })
      x += it.w + gap
    }
    y += heights[i] + gap
  })
  return { w, h, pos }
}

/**
 * Left-to-right tree: depth is the column, each node hangs under one parent
 * (ownership first, then dependency, then reference). Many leaf steps or
 * files under one owner are packed into a block instead of a tall column.
 * Sets x/y as the node centre (nodes carry w/h); returns the balls.
 * @param {{ key: string, depth: number, type: string, title?: string, w: number, h: number, x?: number, y?: number, group?: string }[]} nodes
 * @param {{ from: string, to: string, kind: string }[]} edges
 * @param {{ hgap?: number, vgap?: number, balls?: boolean }} [opts]
 * @returns {{ key: string, owner: string, type: string, title: string, x: number, y: number, w: number, h: number, count: number }[]}
 */
export function layoutTree(nodes, edges, { hgap = 70, vgap = 14, balls = true } = {}) {
  const byKey = new Map(nodes.map((n) => [n.key, n]))
  // Several roots (extra centres on the map) become separate trees, stacked.
  const roots = nodes.filter((n) => n.depth === 0)
  const root = roots[0] ?? nodes[0]
  if (!root) return []
  const isRoot = new Set(roots.length ? roots : [root])
  for (const n of nodes) delete n.group
  const parentOf = new Map()
  for (const e of edges) {
    for (const [c, p] of [
      [e.to, e.from],
      [e.from, e.to],
    ]) {
      const cn = byKey.get(c)
      const pn = byKey.get(p)
      if (!cn || !pn || cn.depth !== pn.depth + 1) continue
      const rank = EDGE_RANK[e.kind] ?? 3
      const cur = parentOf.get(c)
      if (!cur || rank < cur.rank) parentOf.set(c, { p, rank })
    }
  }
  /** @type {Map<string, any[]>} */
  const kids = new Map(nodes.map((n) => [n.key, []]))
  for (const n of nodes) {
    if (isRoot.has(n)) continue
    const par = parentOf.get(n.key)?.p ?? root.key
    kids.get(par)?.push(n)
  }
  const sortKids = (list) =>
    list.sort(
      (a, b) =>
        (TYPE_ORDER[a.type] ?? 9) - (TYPE_ORDER[b.type] ?? 9) ||
        // Steps keep the order they are worked in; the rest sort by title.
        (a.type === 'step' && b.type === 'step' ? (a.sort ?? 0) - (b.sort ?? 0) : 0) ||
        String(a.title ?? '').localeCompare(String(b.title ?? ''), 'ru'),
    )
  for (const list of kids.values()) sortKids(list)

  // Balls: leaf steps / files of one owner, per type.
  const groups = []
  const all = [...nodes]
  if (balls) {
    for (const owner of nodes) {
      const list = kids.get(owner.key) ?? []
      for (const type of BALL_TYPES) {
        const leaves = list.filter((c) => c.type === type && !(kids.get(c.key)?.length))
        if (leaves.length < BALL_MIN) continue
        const pack = packBall(leaves)
        const g = {
          key: `ball:${owner.key}:${type}`,
          owner: owner.key,
          type,
          title: type === 'step' ? 'Шаги' : 'Вложения',
          x: 0,
          y: 0,
          w: pack.w,
          h: pack.h,
          count: leaves.length,
          depth: owner.depth + 1,
          pack,
        }
        groups.push(g)
        const drop = new Set(leaves)
        kids.set(
          owner.key,
          list.filter((c) => !drop.has(c)),
        )
        kids.get(owner.key).push(g)
        kids.set(g.key, [])
        for (const m of leaves) m.group = g.key
        all.push(g)
      }
      sortKids(kids.get(owner.key))
    }
  }
  const sortType = (g) => (g.pack ? g.type : g.type)
  void sortType

  const span = new Map()
  const measure = (n) => {
    const list = kids.get(n.key) ?? []
    let total = 0
    for (const c of list) total += measure(c)
    total += Math.max(0, list.length - 1) * vgap
    const h = Math.max(n.h, total)
    span.set(n.key, h)
    return h
  }
  for (const r of isRoot) measure(r)
  const colW = []
  for (const n of all) colW[n.depth] = Math.max(colW[n.depth] ?? 0, n.w)
  const colX = []
  let acc = 0
  for (let d = 0; d < colW.length; d++) {
    colX[d] = acc
    acc += (colW[d] ?? 0) + hgap
  }
  const place = (n, top) => {
    const h = span.get(n.key)
    const list = kids.get(n.key) ?? []
    n.x = colX[n.depth] + n.w / 2
    if (!list.length) {
      n.y = top + h / 2
    } else {
      const total = list.reduce((s, c) => s + span.get(c.key), 0) + (list.length - 1) * vgap
      let y = top + (h - total) / 2
      for (const c of list) {
        place(c, y)
        y += span.get(c.key) + vgap
      }
      const first = list[0]
      const last = list[list.length - 1]
      n.y = ((first.y ?? 0) + (last.y ?? 0)) / 2
    }
    if (n.pack) {
      for (const p of n.pack.pos) {
        p.item.x = n.x + p.dx
        p.item.y = n.y + p.dy
      }
    }
  }
  let top = 0
  for (const r of isRoot) {
    place(r, top)
    top += span.get(r.key) + vgap * 3
  }
  return groups.map(({ pack, depth, ...g }) => g)
}

/**
 * The same tree, top to bottom: the layout runs on transposed boxes (no
 * balls), then the axes are swapped back.
 * @param {any[]} nodes @param {any[]} edges
 */
export function layoutTreeDown(nodes, edges) {
  for (const n of nodes) [n.w, n.h] = [n.h, n.w]
  layoutTree(nodes, edges, { hgap: 50, vgap: 22, balls: false })
  for (const n of nodes) {
    ;[n.w, n.h] = [n.h, n.w]
    ;[n.x, n.y] = [n.y, n.x]
  }
  return []
}

/**
 * Columns by kind of thing (questlines, quests, steps, notes, files, templates),
 * each stacked top to bottom: shows what the map is made of, not how it hangs.
 * @param {any[]} nodes
 */
export function layoutLayers(nodes, { hgap = 90, vgap = 14 } = {}) {
  const ORDER = ['questline', 'quest', 'step', 'note', 'attachment', 'template']
  const cols = new Map(ORDER.map((t) => [t, []]))
  for (const n of nodes) (cols.get(n.type) ?? cols.get('template')).push(n)
  let x = 0
  for (const t of ORDER) {
    const list = cols.get(t)
    if (!list.length) continue
    list.sort((a, b) => a.depth - b.depth || String(a.title ?? '').localeCompare(String(b.title ?? ''), 'ru'))
    const w = Math.max(...list.map((n) => n.w))
    let y = 0
    for (const n of list) {
      n.x = x + w / 2
      n.y = y + n.h / 2
      y += n.h + vgap
    }
    // Centre the column on the middle of the tallest one afterwards.
    cols.set(t, { list, h: y - vgap })
    x += w + hgap
  }
  const tallest = Math.max(...[...cols.values()].map((c) => c.h ?? 0))
  for (const c of cols.values()) if (c.list) for (const n of c.list) n.y += (tallest - c.h) / 2
  return []
}

/**
 * Rings around the centre by depth; each branch gets an angular sector as wide
 * as its number of leaves. Several centres sit side by side.
 * @param {any[]} nodes @param {any[]} edges
 */
export function layoutRadial(nodes, edges, { gap = 50 } = {}) {
  const byKey = new Map(nodes.map((n) => [n.key, n]))
  const roots = nodes.filter((n) => n.depth === 0)
  if (!roots.length) return []
  const isRoot = new Set(roots)
  const parentOf = new Map()
  for (const e of edges) {
    for (const [c, p] of [
      [e.to, e.from],
      [e.from, e.to],
    ]) {
      const cn = byKey.get(c)
      const pn = byKey.get(p)
      if (!cn || !pn || cn.depth !== pn.depth + 1) continue
      const rank = EDGE_RANK[e.kind] ?? 3
      const cur = parentOf.get(c)
      if (!cur || rank < cur.rank) parentOf.set(c, { p, rank })
    }
  }
  const kids = new Map(nodes.map((n) => [n.key, []]))
  for (const n of nodes) {
    if (isRoot.has(n)) continue
    kids.get(parentOf.get(n.key)?.p ?? roots[0].key)?.push(n)
  }
  for (const list of kids.values()) {
    list.sort((a, b) => (TYPE_ORDER[a.type] ?? 9) - (TYPE_ORDER[b.type] ?? 9) || String(a.title ?? '').localeCompare(String(b.title ?? ''), 'ru'))
  }
  const weight = new Map()
  const wOf = (n) => {
    const list = kids.get(n.key) ?? []
    const w = list.length ? list.reduce((s, c) => s + wOf(c), 0) : 1
    weight.set(n.key, w)
    return w
  }
  let offsetX = 0
  for (const root of roots) {
    wOf(root)
    // Radius per ring: far enough for the widest card, and for the ring's circumference to hold its cards.
    const rings = []
    const collect = (n) => {
      ;(rings[n.depth] ??= []).push(n)
      for (const c of kids.get(n.key) ?? []) collect(c)
    }
    collect(root)
    const radius = [0]
    for (let d = 1; d < rings.length; d++) {
      const list = rings[d] ?? []
      const diag = Math.max(...list.map((n) => Math.hypot(n.w, n.h) / 2), 0)
      const prev = Math.max(...(rings[d - 1] ?? []).map((n) => Math.hypot(n.w, n.h) / 2), 0)
      const around = list.reduce((s, n) => s + Math.max(n.w, n.h) + gap * 0.4, 0) / (2 * Math.PI)
      radius[d] = Math.max(radius[d - 1] + prev + diag + gap, around)
    }
    const place = (n, a0, a1) => {
      const a = (a0 + a1) / 2
      n.x = offsetX + Math.cos(a) * radius[n.depth]
      n.y = Math.sin(a) * radius[n.depth]
      const list = kids.get(n.key) ?? []
      const total = weight.get(n.key)
      let at = a0
      for (const c of list) {
        const span = ((a1 - a0) * weight.get(c.key)) / total
        place(c, at, at + span)
        at += span
      }
    }
    // Start at the left so a lone branch fans out to the right.
    place(root, -Math.PI, Math.PI)
    root.x = offsetX
    root.y = 0
    const reach = Math.max(...nodes.filter((n) => n.depth >= 0).map((n) => Math.abs(n.x - offsetX) + n.w / 2), 0)
    offsetX += reach * 2 + gap * 3
  }
  return []
}
