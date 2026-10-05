<script>
  import { tick } from 'svelte'
  import ContextMenu from '../ui/ContextMenu.svelte'
  import Icon from '../ui/Icon.svelte'
  import MarkdownBody from '../ui/MarkdownBody.svelte'
  import FileKindIcon from '../ui/FileKindIcon.svelte'
  import ConfirmModal from '../modals/ConfirmModal.svelte'
  import QuestModal from '../modals/QuestModal.svelte'
  import DraftEntityModal from '../modals/DraftEntityModal.svelte'
  import FindEntityModal from '../modals/FindEntityModal.svelte'
  import {
    addGraphDraftOp,
    cancelGraphDraft,
    commitGraphDraft,
    deleteGraphDraftOp,
    getGraph,
    getGraphDraft,
    putGraphDraftRoots,
    uploadGraphDraftFile,
  } from '../js/api.js'
  import { toast } from '../js/toasts.svelte.js'
  import { applyDraft, describeOp } from '../js/graphDraft.js'
  import { layoutGraph, layoutTree } from '../js/graphLayout.js'

  /** Relations map tab: what a quest/note is connected to, drawn from existing
   * ownership, step order and reference tokens. Nodes are HTML cards (so
   * markdown renders and long text scrolls); the SVG layer only draws edges.
   * @type {{
   *   active?: boolean,
   *   rootType?: 'quest' | 'note' | 'questline' | null,
   *   rootId?: number | null,
   *   nonce?: number,
   *   quests?: any[],
   *   notes?: any[],
   *   onCenter?: (type: string, id: number) => void,
   *   onOpen?: (kind: string, id: number) => void,
   * }} */
  let {
    active = true,
    rootType = null,
    rootId = null,
    nonce = 0,
    quests = [],
    notes = [],
    onCenter,
    onOpen,
  } = $props()

  const TYPES = {
    quest: { label: 'Квест', glyph: '◆', color: '#c9a227' },
    step: { label: 'Шаг', glyph: '•', color: '#8c97a8' },
    note: { label: 'Заметка', glyph: '¶', color: '#b48ad6' },
    attachment: { label: 'Вложение', glyph: '⎘', color: '#4f9fc7' },
    questline: { label: 'Квестлайн', glyph: '☰', color: '#4fa56b' },
    template: { label: 'Шаблон', glyph: '⟳', color: '#d9894a' },
    cluster: { label: 'Вложения', glyph: '⎘', color: '#4f9fc7' },
  }
  const EDGES = {
    structure: { label: 'Принадлежность', color: '#6e6e6e', dash: '' },
    reference: { label: 'Ссылка в тексте', color: '#c9a227', dash: '6 4' },
    dependency: { label: 'Зависимость', color: '#d0699b', dash: '' },
  }
  /** Attachments of one owner collapse into a cluster beyond this many. */
  const CLUSTER_MIN = 6
  /** Card width / max height (px) by kind; text beyond the height scrolls. */
  const CARD = {
    quest: { w: 300, max: 260 },
    note: { w: 320, max: 300 },
    stepBody: { w: 240, max: 120 },
  }
  const SQUARE = 96
  const RASTER = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])

  let depth = $state(2)
  let layoutMode = $state(/** @type {'tree' | 'free'} */ ('tree'))
  let center = $state({ type: rootType, id: rootId })
  let data = $state(/** @type {any | null} */ (null))
  let loading = $state(false)
  let error = $state('')
  let selectedKey = $state(/** @type {string | null} */ (null))
  let showEdge = $state({ structure: true, reference: true, dependency: true })
  let expanded = $state(/** @type {Set<string>} */ (new Set()))
  let seq = 0

  // --- Draft: edits made here stay in a server-side log until saved. ---
  let draft = $state({ ops: /** @type {any[]} */ ([]), roots: /** @type {{ type: string, id: number }[]} */ ([]) })
  let draftOpen = $state(false)
  let committing = $state(false)
  /** op id → message, from a refused or failed save */
  let opProblems = $state(/** @type {Record<number, string>} */ ({}))
  let cancelAsk = $state(false)
  let dropAsk = $state(/** @type {{ id: number, ids: number[] } | null} */ (null))
  let reloadTick = $state(0)

  async function loadDraft() {
    try {
      const d = await getGraphDraft()
      draft = { ops: d.ops ?? [], roots: Array.isArray(d.roots) ? d.roots : [] }
    } catch {
      /* the page still works read-only */
    }
  }

  /** Adds an operation; resolves to the op, or null when the server refused it. */
  async function addOp(kind, payload) {
    try {
      const op = await addGraphDraftOp(kind, payload)
      await loadDraft()
      return op
    } catch {
      return null
    }
  }

  async function saveDraft() {
    if (committing || !draft.ops.length) return
    committing = true
    opProblems = {}
    try {
      const r = await commitGraphDraft()
      toast(`Сохранено: ${r.applied} ${r.applied === 1 ? 'изменение' : 'изменений'}`, { kind: 'success' })
      saved.clear()
      await loadDraft()
      reloadTick += 1
    } catch (e) {
      const problems = {}
      // @ts-ignore
      for (const p of e.problems ?? []) problems[p.op] = p.message
      // @ts-ignore
      if (e.failedOp != null) problems[e.failedOp] = e.message
      opProblems = problems
      draftOpen = true
      toast(e.message || 'Не удалось сохранить', { kind: 'error' })
      await loadDraft()
      reloadTick += 1
    } finally {
      committing = false
    }
  }

  async function discardDraft() {
    cancelAsk = false
    try {
      await cancelGraphDraft()
      opProblems = {}
      saved.clear()
      await loadDraft()
    } catch {
      /* toast already shown */
    }
  }

  /** Ask which ops would go with this one, then drop them. */
  async function askDrop(op) {
    try {
      const r = await deleteGraphDraftOp(op.id, true)
      const ids = r?.would_remove ?? [op.id]
      if (ids.length > 1) dropAsk = { id: op.id, ids }
      else await doDrop(op.id)
    } catch {
      /* toast already shown */
    }
  }
  async function doDrop(id) {
    dropAsk = null
    try {
      await deleteGraphDraftOp(id)
      await loadDraft()
    } catch {
      /* toast already shown */
    }
  }

  // Camera: pan (x, y) and zoom (k) of the world.
  let cam = $state({ x: 0, y: 0, k: 1 })
  let stageEl = $state(/** @type {HTMLDivElement | null} */ (null))
  /** Laid-out view model: nodes with x/y (centre) and w/h, edges. */
  let view = $state(/** @type {{ nodes: any[], edges: any[], groups?: any[] } | null} */ (null))
  /** False while cards are being measured (they are rendered hidden). */
  let ready = $state(false)

  $effect(() => {
    if (!active || !rootType || !rootId) return
    center = { type: rootType, id: rootId }
    selectedKey = null
    expanded = new Set()
    saved.clear()
  })

  $effect(() => {
    if (active) void loadDraft()
  })

  $effect(() => {
    if (!active || !center.type || !center.id) return
    const c = center
    const d = depth
    const extra = draft.roots.filter((r) => !(r.type === c.type && r.id === c.id))
    void nonce
    void reloadTick
    const mine = ++seq
    loading = true
    error = ''
    getGraph(c.type, c.id, d, extra)
      .then((g) => {
        if (mine !== seq) return
        data = g
      })
      .catch((e) => {
        if (mine === seq) error = e?.message || String(e)
      })
      .finally(() => {
        if (mine === seq) loading = false
      })
  })

  /** Collapses bulky attachment lists into one cluster node. */
  function prepare(g, open_) {
    const nodes = g.nodes.map((n) => ({ ...n, x: 0, y: 0, w: 0, h: 0 }))
    const edges = g.edges.map((e) => ({ ...e }))
    const byKey = new Map(nodes.map((n) => [n.key, n]))
    const owners = new Map()
    for (const e of edges) {
      const to = byKey.get(e.to)
      if (e.kind === 'structure' && to?.type === 'attachment' && to.links === 0 && !to.draft && !e.draft) {
        if (!owners.has(e.from)) owners.set(e.from, [])
        owners.get(e.from).push(to.key)
      }
    }
    const drop = new Set()
    for (const [owner, keys] of owners) {
      const ck = `cluster:${owner}`
      if (keys.length < CLUSTER_MIN || open_.has(ck)) continue
      for (const k of keys) drop.add(k)
      nodes.push({
        key: ck,
        type: 'cluster',
        id: 0,
        title: `${keys.length} вложений`,
        depth: (byKey.get(owner)?.depth ?? 0) + 1,
        links: 0,
        x: 0,
        y: 0,
        w: 0,
        h: 0,
      })
      edges.push({ from: owner, to: ck, kind: 'structure' })
    }
    return {
      nodes: nodes.filter((n) => !drop.has(n.key)),
      edges: edges.filter((e) => !drop.has(e.from) && !drop.has(e.to)),
    }
  }

  /** Positions the user has set by hand (dragged cards), kept across rebuilds.
   * A key like `cluster:<owner>` also remembers where a folded attachment
   * group sat, so an expanded block lands there and folds back to the same spot. */
  const saved = new Map()

  let buildSeq = 0
  /** Render the cards hidden, measure them, lay out, reveal. */
  async function rebuild(g, open_) {
    const mine = ++buildSeq
    ready = false
    view = prepare(g, open_)
    await tick()
    await tick()
    if (mine !== buildSeq || !stageEl || !view) return
    for (const n of view.nodes) {
      const el = stageEl.querySelector(`[data-gkey="${CSS.escape(n.key)}"]`)
      if (el instanceof HTMLElement) {
        n.w = el.offsetWidth
        n.h = el.offsetHeight
      }
    }
    applyLayout()
    ready = true
    await tick()
    // Re-fit only for a new centre / depth, not when a group is folded or the
    // data refreshes: the camera must stay where the user left it.
    const fitKey = `${g.root}|${depth}`
    if (fitKey !== lastFitKey) {
      lastFitKey = fitKey
      fit()
    }
  }
  let lastFitKey = ''

  function applyLayout() {
    if (!view) return
    const attached = view.nodes.filter((n) => n.depth >= 0)
    const floating = view.nodes.filter((n) => n.depth < 0)
    const keys = new Set(attached.map((n) => n.key))
    const edges = view.edges.filter((e) => keys.has(e.from) && keys.has(e.to))
    let groups = []
    if (layoutMode === 'tree') groups = layoutTree(attached, edges)
    else {
      for (const n of view.nodes) delete n.group
      layoutGraph(attached, edges, { sizeOf: (n) => ({ w: n.w, h: n.h }) })
    }
    // Unlinked new things: where they were created, else a column left of the map.
    const left = attached.length ? Math.min(...attached.map((n) => n.x - n.w / 2)) : 0
    let y = attached.length ? Math.min(...attached.map((n) => n.y - n.h / 2)) : 0
    for (const n of floating) {
      if (n.pos) {
        n.x = n.pos.x
        n.y = n.pos.y
      } else {
        n.x = left - 260
        n.y = y + n.h / 2
        y += n.h + 18
      }
    }
    for (const n of attached) {
      if (n.draft && n.pos && !saved.has(n.key)) {
        n.x = n.pos.x
        n.y = n.pos.y
      }
    }
    restoreSaved(groups)
    view = { ...view, groups }
  }

  /** New steps / files with no owner: drawn red, they block saving. */
  let ownerless = $state(/** @type {Set<string>} */ (new Set()))

  // Rebuild when the data, the draft, or the set of expanded groups changes.
  $effect(() => {
    const g = data
    const open_ = expanded
    const ops = draft.ops
    if (!g) return
    const over = applyDraft(g, ops)
    ownerless = over.ownerless
    void rebuild({ ...g, nodes: over.nodes, edges: over.edges }, open_)
  })

  /** Puts hand-placed cards back, and an expanded block where its cluster was. */
  function restoreSaved(groups) {
    if (!view) return
    const handPlaced = new Set()
    for (const n of view.nodes) {
      const p = saved.get(n.key)
      if (p) {
        n.x = p.x
        n.y = p.y
        handPlaced.add(n.key)
      }
    }
    for (const g of groups) {
      const spot = g.type === 'attachment' ? saved.get(`cluster:${g.owner}`) : null
      const members = view.nodes.filter((n) => n.group === g.key)
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
      const shift = freeShift(g, members, groups)
      for (const m of members) {
        m.x += shift.dx
        m.y += shift.dy
      }
    }
  }

  const FRAME = { left: 14, right: 14, top: 44, bottom: 14 }
  const frameOf = (ms) => ({
    x0: Math.min(...ms.map((m) => m.x - m.w / 2)) - FRAME.left,
    x1: Math.max(...ms.map((m) => m.x + m.w / 2)) + FRAME.right,
    y0: Math.min(...ms.map((m) => m.y - m.h / 2)) - FRAME.top,
    y1: Math.max(...ms.map((m) => m.y + m.h / 2)) + FRAME.bottom,
  })

  /** Smallest nudge (down first, then sideways/up) that puts a block's frame
   * clear of every other card and group frame. {0,0} if it is already clear
   * or nothing nearby is free. */
  function freeShift(g, members, allGroups) {
    if (!view) return { dx: 0, dy: 0 }
    const mine = new Set(members.map((m) => m.key))
    const gap = 18
    const obstacles = []
    for (const n of view.nodes) {
      if (mine.has(n.key)) continue
      obstacles.push({ x0: n.x - n.w / 2, x1: n.x + n.w / 2, y0: n.y - n.h / 2, y1: n.y + n.h / 2 })
    }
    for (const og of allGroups) {
      if (og.key === g.key) continue
      const ms = view.nodes.filter((n) => n.group === og.key)
      if (ms.length) obstacles.push(frameOf(ms))
    }
    const f = frameOf(members)
    const clear = (dx, dy) =>
      !obstacles.some(
        (o) => f.x0 + dx < o.x1 + gap && f.x1 + dx > o.x0 - gap && f.y0 + dy < o.y1 + gap && f.y1 + dy > o.y0 - gap,
      )
    if (clear(0, 0)) return { dx: 0, dy: 0 }
    // Directions in order of preference: down, right, up, left, then diagonals.
    const dirs = [
      [0, 1], [1, 0], [0, -1], [-1, 0], [1, 1], [-1, 1], [1, -1], [-1, -1],
    ]
    for (let r = 40; r <= 3000; r += 40) {
      for (const [ux, uy] of dirs) {
        if (clear(ux * r, uy * r)) return { dx: ux * r, dy: uy * r }
      }
    }
    return { dx: 0, dy: 0 }
  }

  function relayout() {
    saved.clear()
    applyLayout()
    void tick().then(fit)
  }

  function fit() {
    if (!view || !stageEl) return
    const r = stageEl.getBoundingClientRect()
    if (!r.width || !view.nodes.length) return
    let x0 = Infinity,
      y0 = Infinity,
      x1 = -Infinity,
      y1 = -Infinity
    for (const n of view.nodes) {
      x0 = Math.min(x0, n.x - n.w / 2)
      x1 = Math.max(x1, n.x + n.w / 2)
      y0 = Math.min(y0, n.y - n.h / 2)
      y1 = Math.max(y1, n.y + n.h / 2)
    }
    const k = Math.min(1, (r.width - 40) / Math.max(1, x1 - x0), (r.height - 40) / Math.max(1, y1 - y0))
    cam = { k, x: r.width / 2 - ((x0 + x1) / 2) * k, y: r.height / 2 - ((y0 + y1) / 2) * k }
  }

  /** Straight run at the end of an edge, so the arrowhead sits on the line. */
  const STUB = 16

  /** Which sides of a and b an edge uses: facing sides, horizontal if far apart in x. */
  function sidesOf(a, b) {
    const dx = b.x - a.x
    if (Math.abs(dx) > (a.w + b.w) / 2 * 0.7) {
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
  function edgePath(a, b, oa = 0, ob = 0) {
    const { vertical, dir } = sidesOf(a, b)
    if (!vertical) {
      const x1 = a.x + (dir * a.w) / 2
      const y1 = a.y + oa
      const x2 = b.x - (dir * b.w) / 2
      const y2 = b.y + ob
      const ex = x2 - dir * STUB
      const off = Math.max(30, Math.abs(ex - x1) / 2)
      return { d: `M${x1} ${y1} C${x1 + dir * off} ${y1} ${ex - dir * off} ${y2} ${ex} ${y2} L${x2} ${y2}`, sx: x1, sy: y1 }
    }
    const x1 = a.x + oa
    const y1 = a.y + (dir * a.h) / 2
    const x2 = b.x + ob
    const y2 = b.y - (dir * b.h) / 2
    const ey = y2 - dir * STUB
    const off = Math.max(30, Math.abs(ey - y1) / 2)
    return { d: `M${x1} ${y1} C${x1} ${y1 + dir * off} ${x2} ${ey - dir * off} ${x2} ${ey} L${x2} ${y2}`, sx: x1, sy: y1 }
  }

  /**
   * Gives every edge end its own point on the card side: outgoing edges share
   * the upper/left half, incoming the lower/right half, each ordered by where
   * the other end is, so lines do not cross needlessly.
   * @param {{ e: any, a: any, b: any }[]} items
   */
  function spreadEnds(items) {
    /** @type {Map<string, { item: any, end: 'a' | 'b', other: number }[]>} */
    const groups = new Map()
    const push = (key, item, end, other) => {
      if (!groups.has(key)) groups.set(key, [])
      groups.get(key).push({ item, end, other })
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

  let nodeMap = $derived(new Map((view?.nodes ?? []).map((n) => [n.key, n])))
  /** Balls follow their members while those are dragged. */
  let groups = $derived(
    (view?.groups ?? []).map((g) => {
      const ms = (view?.nodes ?? []).filter((n) => n.group === g.key)
      if (!ms.length) return g
      const x0 = Math.min(...ms.map((n) => n.x - n.w / 2)) - 14
      const x1 = Math.max(...ms.map((n) => n.x + n.w / 2)) + 14
      const y0 = Math.min(...ms.map((n) => n.y - n.h / 2)) - 44
      const y1 = Math.max(...ms.map((n) => n.y + n.h / 2)) + 14
      return { ...g, x: (x0 + x1) / 2, y: (y0 + y1) / 2, w: x1 - x0, h: y1 - y0 }
    }),
  )
  /** Cluster key a ball can fold back into (only attachment groups that came from one). */
  const collapseKey = (g) =>
    g.type === 'attachment' && expanded.has(`cluster:${g.owner}`) ? `cluster:${g.owner}` : null
  let groupMap = $derived(new Map(groups.map((g) => [g.key, g])))
  /** Steps that wait for another step hang off that step, not off the quest. */
  let chained = $derived(
    new Set(
      (view?.edges ?? [])
        .filter((e) => e.kind === 'dependency' && e.from.startsWith('step:') && e.to.startsWith('step:'))
        .map((e) => e.to),
    ),
  )
  let drawEdges = $derived.by(() => {
    /** @type {{ e: any, a: any, b: any, oa?: number, ob?: number }[]} */
    const items = []
    for (const e of view?.edges ?? []) {
      if (!showEdge[e.kind]) continue
      const a = nodeMap.get(e.from)
      const b = nodeMap.get(e.to)
      if (!a || !b) continue
      if (e.kind === 'structure' && chained.has(e.to) && a.type === 'quest') continue
      // Owner → ball member is drawn once, to the ball itself.
      if (e.kind === 'structure' && b.group && groupMap.get(b.group)?.owner === a.key) continue
      items.push({ e, a, b })
    }
    if (showEdge.structure) {
      for (const g of groups) {
        const o = nodeMap.get(g.owner)
        if (o) items.push({ e: { from: g.owner, to: g.key, kind: 'structure' }, a: o, b: g })
      }
    }
    spreadEnds(items)
    return items.map((it) => ({ ...it.e, ...edgePath(it.a, it.b, it.oa ?? 0, it.ob ?? 0) }))
  })
  let selected = $derived(selectedKey ? (nodeMap.get(selectedKey) ?? null) : null)

  // Pan / zoom / card drag. Dragging a card works from anywhere except its
  // scrollable text and links, so reading and scrolling are not hijacked.
  let drag = null
  function onBgDown(e) {
    if (e.button !== 0 || e.target.closest('[data-gkey]')) return
    drag = { kind: 'pan', x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y }
    stageEl?.setPointerCapture(e.pointerId)
  }
  function onCardDown(e, n) {
    if (e.button !== 0) return
    if (e.target.closest('.gcard__body a, .gcard__body button, .gcard__body pre')) return
    const body = e.target.closest('.gcard__body')
    // Inside the text area: let selection/scrolling happen; only the frame drags.
    if (body && body.scrollHeight > body.clientHeight + 1 && e.offsetX > body.clientWidth - 14) return
    e.stopPropagation()
    drag = { kind: 'node', n, x: e.clientX, y: e.clientY, nx: n.x, ny: n.y, moved: false }
    stageEl?.setPointerCapture(e.pointerId)
  }
  /** Dragging an expanded block by its frame moves all its cards together. */
  function onGroupDown(e, g) {
    if (e.button !== 0) return
    e.stopPropagation()
    const members = (view?.nodes ?? []).filter((n) => n.group === g.key)
    drag = {
      kind: 'group',
      members: members.map((m) => ({ m, x: m.x, y: m.y })),
      x: e.clientX,
      y: e.clientY,
      moved: false,
    }
    stageEl?.setPointerCapture(e.pointerId)
  }
  function onMove(e) {
    if (!drag) return
    const dx = e.clientX - drag.x
    const dy = e.clientY - drag.y
    if (drag.kind === 'pan') {
      cam = { ...cam, x: drag.cx + dx, y: drag.cy + dy }
    } else if (drag.kind === 'group') {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      for (const s of drag.members) {
        s.m.x = s.x + dx / cam.k
        s.m.y = s.y + dy / cam.k
      }
      view = view && { ...view }
    } else {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      drag.n.x = drag.nx + dx / cam.k
      drag.n.y = drag.ny + dy / cam.k
      view = view && { ...view }
    }
  }
  function onUp() {
    if (drag?.kind === 'group' && drag.moved) {
      for (const s of drag.members) saved.set(s.m.key, { x: s.m.x, y: s.m.y })
    }
    if (drag?.kind === 'node' && drag.moved) saved.set(drag.n.key, { x: drag.n.x, y: drag.n.y })
    if (drag?.kind === 'node' && !drag.moved) select(drag.n)
    drag = null
  }
  function onWheel(e) {
    // Over a card's scrollable text the wheel scrolls the text, not the map.
    const body = e.target instanceof Element ? e.target.closest('.gcard__body') : null
    if (body && body.scrollHeight > body.clientHeight + 1) return
    e.preventDefault()
    const r = stageEl?.getBoundingClientRect()
    if (!r) return
    const k = Math.max(0.2, Math.min(2.5, cam.k * (e.deltaY < 0 ? 1.12 : 1 / 1.12)))
    const px = e.clientX - r.left
    const py = e.clientY - r.top
    cam = { k, x: px - ((px - cam.x) / cam.k) * k, y: py - ((py - cam.y) / cam.k) * k }
  }

  /** Folds an expanded attachment group back into its cluster node. */
  /** Centre of a group's cards (not of its frame, which has a header strip). */
  function membersCenter(g) {
    const ms = (view?.nodes ?? []).filter((n) => n.group === g.key)
    if (!ms.length) return { x: g.x, y: g.y }
    return {
      x: (Math.min(...ms.map((m) => m.x - m.w / 2)) + Math.max(...ms.map((m) => m.x + m.w / 2))) / 2,
      y: (Math.min(...ms.map((m) => m.y - m.h / 2)) + Math.max(...ms.map((m) => m.y + m.h / 2))) / 2,
    }
  }

  function collapse(key) {
    const g = groups.find((x) => x.type === 'attachment' && `cluster:${x.owner}` === key)
    if (g) saved.set(key, membersCenter(g))
    expanded = new Set([...expanded].filter((k) => k !== key))
  }

  function collapseAll() {
    for (const k of expanded) {
      const g = groups.find((x) => x.type === 'attachment' && `cluster:${x.owner}` === k)
      if (g) saved.set(k, membersCenter(g))
    }
    expanded = new Set()
  }

  function select(n) {
    if (n.type === 'cluster') {
      saved.set(n.key, { x: n.x, y: n.y })
      expanded = new Set([...expanded, n.key])
      return
    }
    selectedKey = n.key
  }

  /** Where "Открыть" leads: a step opens its quest. */
  function openTarget(n) {
    if (!n || n.draft) return null
    if (n.type === 'quest' || n.type === 'note' || n.type === 'attachment') return [n.type, n.id]
    if (n.type === 'step') {
      const e = data?.edges.find((x) => x.to === n.key && x.kind === 'structure')
      const q = e && data.nodes.find((m) => m.key === e.from)
      return q ? ['quest', q.id] : null
    }
    return null
  }

  function recenter(n) {
    if (n.type === 'step' || n.type === 'cluster' || n.draft) return
    saved.clear()
    center = { type: n.type, id: n.id }
    selectedKey = null
    onCenter?.(n.type, n.id)
  }

  // Root picker: quests and notes by title.
  let rootQuery = $state('')
  let pickOpen = $state(false)
  let matches = $derived.by(() => {
    const q = rootQuery.trim().toLowerCase()
    if (!q) return []
    const out = []
    for (const x of quests) if ((x.title || '').toLowerCase().includes(q)) out.push({ type: 'quest', id: x.id, title: x.title })
    for (const x of notes) if ((x.title || '').toLowerCase().includes(q)) out.push({ type: 'note', id: x.id, title: x.title })
    return out.slice(0, 10)
  })
  function chooseRoot(m) {
    rootQuery = ''
    pickOpen = false
    saved.clear()
    center = { type: m.type, id: m.id }
    selectedKey = null
    onCenter?.(m.type, m.id)
  }

  // Right-click menus: on the empty field (create / upload / add to map) and on a card.
  let ctx = $state({ open: false, x: 0, y: 0, node: /** @type {any | null} */ (null), at: { x: 0, y: 0 } })

  /** Screen point → world coordinates (where a new thing should land). */
  function worldAt(e) {
    const r = stageEl?.getBoundingClientRect()
    if (!r) return { x: 0, y: 0 }
    return { x: (e.clientX - r.left - cam.x) / cam.k, y: (e.clientY - r.top - cam.y) / cam.k }
  }

  function onCtx(e, n = null) {
    e.preventDefault()
    e.stopPropagation()
    const real = n && n.type !== 'cluster' ? n : null
    if (real) selectedKey = real.key
    ctx = { open: true, x: e.clientX, y: e.clientY, node: real, at: worldAt(e) }
  }

  const isExtraRoot = (n) => n && !n.draft && draft.roots.some((r) => `${r.type}:${r.id}` === n.key)

  let ctxItems = $derived.by(() => {
    const n = ctx.node
    if (!n) {
      return [
        {
          id: 'create',
          label: 'Создать',
          children: [
            { id: 'new-quest', label: 'Квест' },
            { id: 'new-note', label: 'Заметку' },
            { id: 'new-step', label: 'Шаг' },
          ],
        },
        { id: 'upload', label: 'Загрузить файл…' },
        { id: 'find', label: 'Добавить на карту…' },
      ]
    }
    const items = []
    if (n.draft) {
      items.push({ id: 'drop', label: 'Убрать из черновика', danger: true })
      return items
    }
    if (openTarget(n)) items.push({ id: 'open', label: 'Открыть' })
    if (n.type !== 'step' && n.key !== data?.root) items.push({ id: 'center', label: 'Сделать центром' })
    if (isExtraRoot(n)) items.push({ id: 'unroot', label: 'Убрать с карты' })
    if (n.type === 'quest' || n.type === 'note') items.push({ id: 'new-quest-ref', label: 'Создать квест со ссылкой сюда' })
    return items
  })

  // Modals started from the menus.
  let questModal = $state({ open: false, desc: '', at: { x: 0, y: 0 } })
  let entityModal = $state({ open: false, kind: /** @type {'note' | 'step'} */ ('note'), at: { x: 0, y: 0 } })
  let findModal = $state(false)
  let fileInput = $state(/** @type {HTMLInputElement | null} */ (null))
  let fileAt = { x: 0, y: 0 }

  async function onCtxSelect(id) {
    const n = ctx.node
    const at = ctx.at
    if (id === 'new-quest') questModal = { open: true, desc: '', at }
    else if (id === 'new-quest-ref' && n) questModal = { open: true, desc: `${n.type}=${n.id}`, at }
    else if (id === 'new-note') entityModal = { open: true, kind: 'note', at }
    else if (id === 'new-step') entityModal = { open: true, kind: 'step', at }
    else if (id === 'upload') {
      fileAt = at
      fileInput?.click()
    } else if (id === 'find') findModal = true
    else if (id === 'drop' && n?.op) await askDrop({ id: n.op })
    else if (id === 'unroot' && n) {
      const roots = draft.roots.filter((r) => `${r.type}:${r.id}` !== n.key)
      await putGraphDraftRoots(roots)
      await loadDraft()
    } else if (id === 'open' && n) {
      const t = openTarget(n)
      if (t) onOpen?.(t[0], t[1])
    } else if (id === 'center' && n) recenter(n)
  }

  async function onFilesPicked(e) {
    const files = [...(e.currentTarget.files ?? [])]
    e.currentTarget.value = ''
    for (const [i, file] of files.entries()) {
      try {
        const op = await uploadGraphDraftFile(file)
        saved.set(`attachment:t${op.id}`, { x: fileAt.x + i * 110, y: fileAt.y })
      } catch {
        /* toast already shown */
      }
    }
    await loadDraft()
  }

  async function addToMap(type, id) {
    if (center.type === type && center.id === id) {
      toast('Это уже центр карты', { kind: 'info' })
      return
    }
    if (draft.roots.some((r) => r.type === type && r.id === id)) {
      toast('Уже на карте', { kind: 'info' })
      return
    }
    await putGraphDraftRoots([...draft.roots, { type, id }])
    await loadDraft()
  }

  const titleOf = (key) => nodeMap.get(key)?.title ?? key

  const isDone = (n) => n.status === 'completed' || n.status === 'done'
  const isOrphan = (n) => (n.type === 'note' || n.type === 'attachment') && n.links === 0
  const rasterOf = (n) => n.type === 'attachment' && RASTER.has(String(n.mime || '').split(';')[0].trim())
</script>

<div class="graph">
  <div class="graph__tools">
    <div class="graph__pick">
      <input
        class="search graph__search"
        type="search"
        placeholder="Центр: найти квест или заметку…"
        bind:value={rootQuery}
        onfocus={() => (pickOpen = true)}
        onblur={() => setTimeout(() => (pickOpen = false), 150)}
        aria-label="Выбрать центр карты"
      />
      {#if pickOpen && matches.length}
        <ul class="graph__matches" role="listbox">
          {#each matches as m (m.type + m.id)}
            <li>
              <button type="button" onmousedown={(e) => e.preventDefault()} onclick={() => chooseRoot(m)}>
                <span style:color={TYPES[m.type].color}>{TYPES[m.type].glyph}</span>
                {m.title}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    <label class="graph__ctl">
      Раскладка
      <select bind:value={layoutMode} onchange={relayout} aria-label="Раскладка">
        <option value="tree">Дерево</option>
        <option value="free">Свободно</option>
      </select>
    </label>
    <label class="graph__ctl">
      Глубина
      <select bind:value={depth} aria-label="Глубина">
        <option value={1}>1</option>
        <option value={2}>2</option>
        <option value={3}>3</option>
      </select>
    </label>
    {#each Object.entries(EDGES) as [k, e] (k)}
      <label class="graph__ctl graph__edge">
        <input type="checkbox" bind:checked={showEdge[k]} />
        <svg width="26" height="8" aria-hidden="true">
          <line x1="0" y1="4" x2="26" y2="4" stroke={e.color} stroke-width="2" stroke-dasharray={e.dash} />
        </svg>
        {e.label}
      </label>
    {/each}
    {#if expanded.size}
      <button type="button" class="graph__btn" onclick={collapseAll}>Свернуть вложения</button>
    {/if}
    <button type="button" class="graph__btn" onclick={relayout}>Распутать</button>
    <button type="button" class="graph__btn" onclick={fit}>Вписать</button>
  </div>

  {#if draft.ops.length}
    <div class="gdraft">
      <button type="button" class="gdraft__toggle" aria-expanded={draftOpen} onclick={() => (draftOpen = !draftOpen)}>
        Черновик · {draft.ops.length}
        <Icon name={draftOpen ? 'chevron-down' : 'chevron-right'} size={12} />
      </button>
      <span class="gdraft__hint">
        {#if ownerless.size}
          Нужно прикрепить к владельцу: {ownerless.size} (красная рамка). Пока не прикреплено, сохранить нельзя.
        {:else}
          В базе пока ничего нет — сохраните или отмените.
        {/if}
      </span>
      <button
        type="button"
        class="graph__btn graph__btn--primary"
        disabled={committing || ownerless.size > 0}
        title={ownerless.size ? 'Есть шаги или файлы без владельца' : 'Применить все изменения'}
        onclick={saveDraft}
      >
        {committing ? 'Сохраняю…' : 'Сохранить'}
      </button>
      <button type="button" class="graph__btn" onclick={() => (cancelAsk = true)}>Отменить всё</button>
    </div>
    {#if draftOpen}
      <ol class="gdraft__list">
        {#each draft.ops as op (op.id)}
          <li class:gdraft__row--bad={opProblems[op.id]}>
            <span class="gdraft__text">{describeOp(op, titleOf)}</span>
            {#if opProblems[op.id]}<span class="gdraft__msg">{opProblems[op.id]}</span>{/if}
            <button type="button" class="gdraft__x" title="Убрать из черновика" aria-label="Убрать из черновика" onclick={() => askDrop(op)}>
              <Icon name="close" size={12} />
            </button>
          </li>
        {/each}
      </ol>
    {/if}
  {/if}

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="graph__stage"
    bind:this={stageEl}
    onpointerdown={onBgDown}
    onpointermove={onMove}
    onpointerup={onUp}
    onpointercancel={onUp}
    onwheel={onWheel}
    oncontextmenu={(e) => onCtx(e)}
  >
    {#if !center.id}
      <p class="graph__msg">Выберите квест или заметку в поиске выше — или откройте карту из карточки квеста.</p>
    {:else if error}
      <p class="graph__msg">{error}</p>
    {:else if loading && !view}
      <p class="graph__msg">Загрузка…</p>
    {/if}

    <div class="gworld" style:transform="translate({cam.x}px, {cam.y}px) scale({cam.k})" class:gworld--hidden={!ready}>
      <svg class="gedges" aria-hidden="true">
        <defs>
          {#each Object.entries(EDGES) as [k, e] (k)}
            <marker id="arrow-{k}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0 0L10 5L0 10z" fill={e.color} />
            </marker>
          {/each}
        </defs>
        {#each drawEdges as e, i (i)}
          <path
            d={e.d}
            fill="none"
            stroke={e.draft === 'remove' ? '#d23c3c' : EDGES[e.kind].color}
            stroke-width={(e.kind === 'structure' ? 1.2 : 1.8) + (e.draft === 'add' ? 1 : 0)}
            stroke-dasharray={e.draft === 'remove' ? '2 5' : EDGES[e.kind].dash}
            opacity={e.draft === 'remove' ? 0.9 : e.kind === 'structure' ? 0.6 : 0.95}
            marker-end={e.kind === 'structure' ? undefined : `url(#arrow-${e.kind})`}
          />
          {#if e.kind !== 'structure'}
            <circle cx={e.sx} cy={e.sy} r="3.4" fill={e.draft === 'remove' ? '#d23c3c' : EDGES[e.kind].color} />
          {/if}
        {/each}
      </svg>

      {#each groups as g (g.key)}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="ggroup"
          style:left="{g.x - g.w / 2}px"
          style:top="{g.y - g.h / 2}px"
          style:width="{g.w}px"
          style:height="{g.h}px"
          style:--c={TYPES[g.type].color}
          onpointerdown={(e) => onGroupDown(e, g)}
        >
          {#if collapseKey(g)}
            <button
              type="button"
              class="ggroup__label ggroup__label--btn"
              title="Свернуть обратно"
              onpointerdown={(e) => e.stopPropagation()}
              onclick={() => collapse(collapseKey(g) ?? '')}
            >
              <span class="ggroup__fold" aria-hidden="true"><Icon name="chevron-down" size={12} /></span>
              {g.title} · {g.count} · свернуть
            </button>
          {:else}
            <span class="ggroup__label">{g.title} · {g.count}</span>
          {/if}
        </div>
      {/each}

      {#each view?.nodes ?? [] as n (n.key)}
        {@const t = TYPES[n.type] ?? TYPES.step}
        {@const square = n.type === 'attachment' || n.type === 'cluster'}
        {@const big = n.type === 'quest' || n.type === 'note'}
        {@const withText = n.type === 'step' && !!n.desc}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="gcard gcard--{n.type}"
          class:gcard--sel={n.key === selectedKey}
          class:gcard--root={n.key === data?.root}
          class:gcard--done={isDone(n)}
          class:gcard--orphan={isOrphan(n)}
          class:gcard--draft={n.draft}
          class:gcard--ownerless={ownerless.has(n.key)}
          class:gcard--pill={!big && !square && !withText}
          style:--c={ownerless.has(n.key) ? `color-mix(in srgb, ${t.color} 35%, #e0443e)` : t.color}
          style:left="{n.x - n.w / 2}px"
          style:top="{n.y - n.h / 2}px"
          style:width={big ? `${CARD[n.type].w}px` : withText ? `${CARD.stepBody.w}px` : square ? `${SQUARE}px` : undefined}
          style:height={ready && n.h ? `${n.h}px` : undefined}
          data-gkey={n.key}
          title="{t.label}: {n.title}{isOrphan(n) ? ' — на неё нет ссылок' : ''}"
          onpointerdown={(e) => onCardDown(e, n)}
          oncontextmenu={(e) => onCtx(e, n)}
        >
          {#if square}
            <div class="gcard__sq">
              {#if rasterOf(n)}
                <img class="gcard__thumb" src="/api/attachments/{n.id}/image?preview=1" alt="" loading="lazy" draggable="false" />
              {:else if n.type === 'cluster'}
                <span class="gcard__count">{n.title.split(' ')[0]}</span>
              {:else}
                <FileKindIcon att={{ filename: n.title, content_type_detected: n.mime }} size={30} />
              {/if}
              <span class="gcard__name">{n.type === 'cluster' ? 'вложений' : n.title}</span>
            </div>
          {:else if big || withText}
            <div class="gcard__head">
              <span class="gcard__glyph">{t.glyph}</span>
              <span class="gcard__title">{n.title}</span>
              {#if n.type === 'quest' && n.status && n.status !== 'active'}
                <span class="gcard__status">{n.status}</span>
              {:else if n.type === 'step' && n.progress}
                <span class="gcard__prog">{n.progress}</span>
              {/if}
            </div>
            {#if n.desc}
              <div class="gcard__body" style:max-height="{big ? CARD[n.type].max : CARD.stepBody.max}px">
                <MarkdownBody source={n.desc} labels={{}} onRef={(k, id) => onOpen?.(k, id)} />
              </div>
            {/if}
          {:else}
            <span class="gcard__glyph">{n.type === 'step' && isDone(n) ? '✓' : t.glyph}</span>
            <span class="gcard__title gcard__title--line">{n.title}</span>
            {#if n.type === 'step' && n.progress}
              <span class="gcard__prog">{n.progress}</span>
            {/if}
          {/if}
        </div>
      {/each}
    </div>

    {#if data?.truncated}
      <p class="graph__trunc">Показаны не все узлы — уменьшите глубину</p>
    {/if}
  </div>

  <div class="graph__foot">
    {#if selected}
      {@const tgt = openTarget(selected)}
      <span class="graph__sel">
        <b>{TYPES[selected.type]?.label}</b>
        {selected.title}
        <span class="graph__meta">· ссылок: {selected.links}</span>
      </span>
      {#if tgt}
        <button type="button" class="graph__btn" onclick={() => onOpen?.(tgt[0], tgt[1])}>Открыть</button>
      {/if}
      {#if selected.key !== data?.root && selected.type !== 'step'}
        <button type="button" class="graph__btn" onclick={() => recenter(selected)}>Сделать центром</button>
      {/if}
    {:else}
      <span class="graph__meta">
        Клик по карточке — подробности, ПКМ — действия. Пунктирная рамка у заметки или вложения — на неё никто не ссылается.
        {#each ['quest', 'step', 'note', 'attachment', 'questline', 'template'] as k (k)}
          <span class="graph__key" style:color={TYPES[k].color}>{TYPES[k].glyph} {TYPES[k].label}</span>
        {/each}
      </span>
    {/if}
  </div>
</div>

<input bind:this={fileInput} type="file" multiple hidden onchange={onFilesPicked} />

<QuestModal
  open={questModal.open}
  mode="create"
  defaults={{ description: questModal.desc }}
  {quests}
  {notes}
  attachments={[]}
  onClose={() => (questModal = { ...questModal, open: false })}
  onSaved={() => {}}
  onDraft={(payload) => addOp('create_quest', { fields: payload, pos: questModal.at })}
/>

<DraftEntityModal
  open={entityModal.open}
  kind={entityModal.kind}
  {quests}
  {notes}
  onClose={() => (entityModal = { ...entityModal, open: false })}
  onSubmit={(fields) => addOp(entityModal.kind === 'note' ? 'create_note' : 'create_step', { fields, pos: entityModal.at })}
/>

<FindEntityModal open={findModal} {quests} {notes} onClose={() => (findModal = false)} onPick={addToMap} />

<ConfirmModal
  open={cancelAsk}
  title="Отменить все изменения?"
  message={`Черновик (${draft.ops.length}) будет сброшен, загруженные в него файлы удалены. В базе ничего не изменится.`}
  confirmLabel="Отменить всё"
  onCancel={() => (cancelAsk = false)}
  onConfirm={discardDraft}
/>

<ConfirmModal
  open={dropAsk != null}
  title="Убрать из черновика?"
  message={`Вместе с этим уберётся то, что на него опирается: всего операций ${dropAsk?.ids.length ?? 0}.`}
  confirmLabel="Убрать"
  onCancel={() => (dropAsk = null)}
  onConfirm={() => dropAsk && doDrop(dropAsk.id)}
/>

<ContextMenu
  open={ctx.open}
  x={ctx.x}
  y={ctx.y}
  items={ctxItems}
  onSelect={onCtxSelect}
  onClose={() => (ctx = { ...ctx, open: false })}
/>

<style>
  .graph {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    height: 100%;
    min-height: 0;
    padding: 0.75rem 1rem 1rem;
    box-sizing: border-box;
  }

  .graph__tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.9rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__ctl {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .graph__ctl select {
    padding: 0.2rem 0.4rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    background: transparent;
    color: inherit;
    font: inherit;
  }

  .graph__pick {
    position: relative;
    flex: 0 1 18rem;
    min-width: 11rem;
  }

  .graph__search {
    width: 100%;
    box-sizing: border-box;
  }

  .graph__matches {
    position: absolute;
    z-index: 5;
    top: calc(100% + 0.25rem);
    left: 0;
    right: 0;
    margin: 0;
    padding: 0.25rem;
    list-style: none;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: var(--pill-shadow, none);
  }

  .graph__matches button {
    display: block;
    width: 100%;
    padding: 0.3rem 0.5rem;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .graph__matches button:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .graph__btn {
    padding: 0.25rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }

  .graph__btn:hover {
    border-color: var(--color-accent, #c9a227);
  }

  .graph__stage {
    position: relative;
    flex: 1 1 auto;
    min-height: 16rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    overflow: hidden;
  }

  .graph__msg,
  .graph__trunc {
    position: absolute;
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__msg {
    inset: 0;
    display: grid;
    place-items: center;
  }

  .graph__trunc {
    left: 0.8rem;
    bottom: 0.6rem;
    font-size: var(--text-xs, 0.75rem);
  }

  .graph__foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
    min-height: 1.8rem;
    font-size: var(--text-sm, 0.875rem);
  }

  .graph__sel {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .graph__meta {
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__key {
    margin-left: 0.6rem;
    white-space: nowrap;
  }

  .graph__stage {
    cursor: grab;
    touch-action: none;
    user-select: none;
  }

  .gworld {
    position: absolute;
    left: 0;
    top: 0;
    width: 0;
    height: 0;
    transform-origin: 0 0;
  }

  .gworld--hidden {
    visibility: hidden;
  }

  .gedges {
    position: absolute;
    left: 0;
    top: 0;
    width: 1px;
    height: 1px;
    overflow: visible;
    pointer-events: none;
  }

  .ggroup {
    position: absolute;
    box-sizing: border-box;
    border: 1.5px solid color-mix(in srgb, var(--c, #888) 45%, transparent);
    border-radius: 18px;
    background: color-mix(in srgb, var(--c, #888) 7%, transparent);
    cursor: grab;
  }

  .ggroup__label {
    position: absolute;
    left: 0.9rem;
    top: 0.55rem;
    font-size: 0.72rem;
    color: var(--c, #888);
    white-space: nowrap;
  }

  .ggroup__label--btn {
    padding: 0.1rem 0.5rem;
    border: 1px solid color-mix(in srgb, var(--c, #888) 55%, transparent);
    border-radius: 999px;
    background: var(--color-bg-raised, #1a1a1a);
    font: inherit;
    font-size: 0.68rem;
    cursor: pointer;
    pointer-events: auto;
  }

  .ggroup__label--btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .ggroup__fold {
    display: inline-flex;
    transform: rotate(180deg);
  }

  .ggroup__label--btn:hover {
    border-color: var(--c, #888);
  }

  .gcard {
    position: absolute;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.55rem 0.7rem;
    border: 2.2px solid var(--c, #888);
    border-radius: 14px;
    /* Card tinted with its type colour, so kind reads at a glance. */
    background: color-mix(in srgb, var(--c, #888) 11%, var(--color-bg-raised, #1a1a1a));
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
    cursor: default;
    user-select: text;
  }

  .gcard--pill {
    flex-direction: row;
    align-items: center;
    gap: 0.4rem;
    max-width: 240px;
    padding: 0.3rem 0.8rem;
    border-radius: 999px;
    white-space: nowrap;
  }

  .gcard--step:not(.gcard--pill) {
    border-radius: 10px;
  }

  .gcard--note {
    border-radius: 10px;
  }

  .gcard--attachment,
  .gcard--cluster {
    padding: 0.4rem;
    border-radius: 18px;
    text-align: center;
  }

  .gcard--root {
    border-width: 3px;
  }

  .gcard--sel {
    background: color-mix(in srgb, var(--c, #c9a227) 24%, var(--color-bg-raised, #1a1a1a));
  }

  .gcard__prog {
    flex: none;
    margin-left: auto;
    padding-left: 0.5rem;
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gcard--done {
    opacity: 0.6;
  }

  .gcard--orphan {
    border-style: dashed;
  }

  /* New, not yet saved: dotted frame; without an owner the frame turns red. */
  .gcard--draft {
    border-style: dotted;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c, #888) 14%, transparent);
  }

  .gcard--ownerless {
    box-shadow: 0 0 0 3px color-mix(in srgb, #e0443e 22%, transparent);
  }

  .gdraft {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.7rem;
    border: 1px solid color-mix(in srgb, var(--color-accent, #c9a227) 55%, transparent);
    border-radius: 10px;
    background: color-mix(in srgb, var(--color-accent, #c9a227) 8%, transparent);
    font-size: var(--text-sm, 0.875rem);
  }

  .gdraft__toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    border: 0;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  .gdraft__hint {
    flex: 1 1 14rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__btn--primary {
    background: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
    color: var(--color-bg, #121212);
    font-weight: 600;
  }

  .graph__btn--primary:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .gdraft__list {
    list-style: none;
    margin: 0;
    padding: 0.2rem 0.4rem;
    max-height: 11rem;
    overflow: auto;
    border: 1px solid var(--color-border, #333);
    border-radius: 10px;
    font-size: var(--text-xs, 0.75rem);
  }

  .gdraft__list li {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.2rem 0.3rem;
  }

  .gdraft__text {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .gdraft__msg {
    color: #e0443e;
  }

  .gdraft__row--bad {
    background: color-mix(in srgb, #e0443e 12%, transparent);
    border-radius: 6px;
  }

  .gdraft__x {
    flex: none;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .gdraft__x:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .gcard__head {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    min-width: 0;
  }

  .gcard__glyph {
    flex: none;
    color: var(--c, #888);
  }

  .gcard__title {
    min-width: 0;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .gcard__title--line {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
    font-size: 0.8rem;
  }

  .gcard__status {
    margin-left: auto;
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gcard__body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    font-size: 0.78rem;
    line-height: 1.45;
    color: var(--color-fg-muted, #b5b5b5);
    cursor: auto;
  }

  .gcard__body :global(p) {
    margin: 0 0 0.4em;
  }

  .gcard__body :global(h1),
  .gcard__body :global(h2),
  .gcard__body :global(h3) {
    margin: 0.4em 0 0.25em;
    font-size: 0.9rem;
  }

  .gcard__body :global(pre) {
    overflow: auto;
  }

  .gcard__body :global(img) {
    max-width: 100%;
  }

  .gcard__sq {
    display: grid;
    justify-items: center;
    align-content: center;
    gap: 0.3rem;
    height: 100%;
    min-width: 0;
  }

  .gcard__thumb {
    width: 100%;
    height: 56px;
    object-fit: cover;
    border-radius: 10px;
    pointer-events: none;
  }

  .gcard__count {
    font-size: 1.5rem;
    font-weight: 600;
    color: var(--c, #888);
  }

  .gcard__name {
    max-width: 100%;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
    font-size: 0.68rem;
    line-height: 1.2;
  }
</style>
