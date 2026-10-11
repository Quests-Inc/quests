<script lang="ts">
  import { tick, untrack } from 'svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import { helpLine } from '@/shared/lib/searchFlags.ts'
  import { ENTITY_SPEC, findEntities } from '@/entities/quest'
  import Icon from '@/shared/ui/Icon.svelte'
  import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import { QuestlineIcon } from '@/entities/questline'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { QuestModal, StepModal, DraftEntityModal } from '@/features/quest-edit'
  import { FindEntityModal } from '@/features/find-entity'
  import {
    addGraphDraftOp,
    cancelGraphDraft,
    commitGraphDraft,
    deleteGraphDraftOp,
    getGraph,
    getGraphDraft,
    getNote,
    QUEST_STATUSES,
    QUEST_STATUS_LABELS,
    getQuestFull,
    putGraphDraftOp,
    putGraphDraftRoots,
    uploadGraphDraftFile,
  } from '@/shared/api/client.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { applyDraft, describeOp, layoutGraph, layoutLayers, layoutRadial, layoutTree, layoutTreeDown } from '@/entities/graph'

  /**
   * Relations map tab: what a quest/note is connected to, drawn from existing
   * ownership, step order and reference tokens. Nodes are HTML cards (so
   * markdown renders and long text scrolls); the SVG layer only draws edges.
   */
  let {
    active = true,
    rootType = null,
    rootId = null,
    nonce = 0,
    quests = [],
    notes = [],
    questlines = [],
    onCenter,
    onOpen,
  }: {
    active?: boolean,
    rootType?: 'quest' | 'note' | 'questline' | null,
    rootId?: number | null,
    nonce?: number,
    quests?: any[],
    notes?: any[],
    questlines?: any[],
    onCenter?: (type: string | null, id: number | null) => void,
    onOpen?: (kind: string, id: number) => void,
  } = $props()

  const TYPES: Record<string, any> = {
    quest: { label: 'Квест', glyph: '◆', color: '#c9a227' },
    step: { label: 'Шаг', glyph: '•', color: '#8c97a8' },
    note: { label: 'Заметка', glyph: '¶', color: '#b48ad6' },
    attachment: { label: 'Вложение', glyph: '⎘', color: '#4f9fc7' },
    questline: { label: 'Квестлайн', glyph: '☰', color: '#4fa56b' },
    template: { label: 'Шаблон', glyph: '⟳', color: '#d9894a' },
    cluster: { label: 'Вложения', glyph: '⎘', color: '#4f9fc7' },
  }
  const EDGES: Record<string, any> = {
    structure: { label: 'Принадлежность', color: '#6e6e6e', dash: '' },
    reference: { label: 'Ссылка', color: '#c9a227', dash: '6 4' },
    dependency: { label: 'Зависимость', color: '#d0699b', dash: '' },
  }
  /** Attachments of one owner collapse into a cluster beyond this many. */
  const CLUSTER_MIN = 6
  /** Card width / max height (px) by kind; text beyond the height scrolls. */
  const CARD: Record<string, any> = {
    quest: { w: 300, max: 260 },
    note: { w: 320, max: 300 },
    stepBody: { w: 240, max: 120 },
  }
  const SQUARE = 96
  const RASTER = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])

  let depth = $state(2)
  const LAYOUTS = {
    tree: 'Дерево',
    down: 'Дерево сверху вниз',
    radial: 'Радиальная',
    layers: 'Слоями по типам',
    free: 'Свободно',
  }
  const LAYOUT_KEY = 'quests.graph.layout'
  let layoutMode = $state(
    (
      (() => {
        try {
          const v = localStorage.getItem(LAYOUT_KEY)
          return v && v in LAYOUTS ? v : 'tree'
        } catch {
          return 'tree'
        }
      })() as keyof typeof LAYOUTS
    ),
  )
  let center = $state(({ type: rootType, id: rootId } as { type: string, id: number }))
  let data = $state((null as any | null))
  let loading = $state(false)
  let error = $state('')
  let selectedKey = $state((null as string | null))
  let showEdge: Record<string, boolean> = $state({ structure: true, reference: true, dependency: true })
  let expanded = $state((new Set() as Set<string>))
  /** Owners whose files are laid out one by one: never a folded group or a block. */
  let loose = $state((new Set() as Set<string>))
  let seq = 0

  // --- Draft: edits made here stay in a server-side log until saved. ---
  let draft = $state({ ops: ([] as any[]), roots: ([] as { type: string, id: number }[]) })
  let draftOpen = $state(true)
  let committing = $state(false)
  /** op id → message, from a refused or failed save */
  let opProblems = $state(({} as Record<number, string>))
  let cancelAsk = $state(false)
  let dropAsk = $state((null as { id: number, ids: number[] } | null))
  let reloadTick = $state(0)

  async function loadDraft() {
    try {
      const d = await getGraphDraft()
      const next = { ops: d.ops ?? [], roots: Array.isArray(d.roots) ? d.roots : [] }
      // Unchanged: keep the object, so nothing is rebuilt.
      if (JSON.stringify(next) !== JSON.stringify(draft)) draft = next
    } catch {
      /* the page still works read-only */
    }
  }

  /** Adds an operation; resolves to the op, or null when the server refused it. */
  async function addOp(kind: string, payload: { id?: any; parent?: any; was?: any; item?: string; owner?: any; from?: any; to?: any; source?: any; pos?: { x: number; y: number }|{ x: number; y: number }|{ x: number; y: number }; fields?: any; quest?: any }) {
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
      // An empty map has nothing to show the saved things on: centre on the first one.
      const made = !center.id ? Object.entries(r.created ?? {}).find(([k]) => k.startsWith('quest:') || k.startsWith('note:') || k.startsWith('questline:')) : null
      // Saving must not rearrange the map: every card stays where it is, and
      // the things that were only in the draft keep their spot under their new ids.
      for (const n of view?.nodes ?? []) {
        if (!Number.isFinite(n.x) || !Number.isFinite(n.y)) continue
        const real = r.created?.[n.key]
        const key = real != null ? `${n.key.split(':')[0]}:${real}` : n.key
        if (n.type !== 'cluster') saved.set(key, { x: n.x, y: n.y })
      }
      if (made) {
        center = { type: made[0].split(':')[0], id: Number(made[1]) }
        onCenter?.(center.type, center.id)
      }
      // Things made on the map with no link to anything on it have nothing to
      // hang from once saved: keep them as extra centres so they stay put.
      const kept = [...draft.roots]
      for (const n of view?.nodes ?? []) {
        if (!n.draft || n.depth >= 0 || !['quest', 'note', 'questline'].includes(n.type)) continue
        const id = Number(r.created?.[n.key])
        if (!id || (center.type === n.type && center.id === id)) continue
        if (!kept.some((x) => x.type === n.type && x.id === id)) kept.push({ type: n.type, id })
      }
      if (kept.length !== draft.roots.length) await putGraphDraftRoots(kept)
      await loadDraft()
      reloadTick += 1
    } catch (e) {
      const problems: Record<number, string> = {}
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
      resetPositions()
      await loadDraft()
    } catch {
      /* toast already shown */
    }
  }

  /** Ask which ops would go with this one, then drop them. */
  async function askDrop(op: { id: any }) {
    try {
      const r = await deleteGraphDraftOp(op.id, true)
      const ids = r?.would_remove ?? [op.id]
      if (ids.length > 1) dropAsk = { id: op.id, ids }
      else await doDrop(op.id)
    } catch {
      /* toast already shown */
    }
  }
  async function doDrop(id: number) {
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
  let stageEl = $state((null as HTMLDivElement | null))
  /** Laid-out view model: nodes with x/y (centre) and w/h, edges. */
  let view = $state((null as { nodes: any[], edges: any[], groups?: any[] } | null))
  /** False while cards are being measured (they are rendered hidden). */
  let ready = $state(false)

  let lastRootKey = ''
  let lastDataJson = ''
  $effect(() => {
    if (!active || !rootType || !rootId) return
    // Coming back to the (kept) page with the same centre changes nothing.
    const rk = `${rootType}:${rootId}:${nonce}`
    if (rk === lastRootKey) return
    lastRootKey = rk
    center = { type: rootType, id: rootId }
    selectedKey = null
    expanded = new Set()
    loose = new Set()
    setExpand([])
    resetPositions()
  })

  $effect(() => {
    if (active) void loadDraft()
  })

  $effect(() => {
    if (!active) return
    // No centre yet: an empty base map, so things made in the draft still show.
    if (!center.type || !center.id) {
      lastDataJson = ''
      data = { root: '', roots: [], nodes: [], edges: [], truncated: false, depth }
      return
    }
    const c = center
    const d = depth
    const extra = draft.roots.filter((r) => !(r.type === c.type && r.id === c.id))
    const open = expandKeys.map((k) => {
      // "quest:5" opens every link of the card, "quest:5>note" only its notes.
      const [ref, only] = k.split('>')
      return { type: ref.split(':')[0], id: Number(ref.split(':')[1]), only }
    })
    void nonce
    void reloadTick
    const mine = ++seq
    loading = true
    error = ''
    getGraph(c.type, c.id, d, extra, open)
      .then((g) => {
        if (mine !== seq) return
        if (JSON.stringify(g) !== lastDataJson) {
          lastDataJson = JSON.stringify(g)
          data = g
        }
      })
      .catch((e) => {
        if (mine === seq) error = e?.message || String(e)
      })
      .finally(() => {
        if (mine === seq) loading = false
      })
  })

  /** Keys that never reach the map: typing in a field, or a dialog on top. */
  const typing = (e: { key?: string; altKey?: any; ctrlKey?: any; metaKey?: any; code?: string; preventDefault?: () => void; shiftKey?: any; target?: any }) =>
    e.target instanceof Element && ((e.target as Element).closest('input, textarea, select, [contenteditable="true"]') || document.querySelector('.backdrop [role="dialog"]'))

  /** Map shortcuts (by physical key, so they work in any keyboard layout):
   * Ctrl+S save, Ctrl+Z undo the last change, V select, F fit, R untangle, Shift+Del clear. */
  function onKey(e: { key: string; altKey: any; ctrlKey: any; metaKey: any; code: string; preventDefault: () => void; shiftKey: any }) {
    if (e.key === 'Escape') {
      if (linking) cancelLink()
      else if (selectMode) toggleSelectMode()
      // An open menu or dialog takes this Esc for itself.
      else if (full && !ctx.open && !typing(e)) toggleFull()
      return
    }
    if (!active || e.altKey) return
    const mod = e.ctrlKey || e.metaKey
    if (mod && e.code === 'KeyS') {
      e.preventDefault()
      if (!ownerless.size && !document.querySelector('.backdrop [role="dialog"]')) void saveDraft()
      return
    }
    if (typing(e)) return
    if (mod && e.code === 'KeyZ' && !e.shiftKey) {
      e.preventDefault()
      const last = draft.ops[draft.ops.length - 1]
      if (last) void askDrop(last)
      return
    }
    if (mod) return
    if (e.code === 'KeyV' && !e.shiftKey) toggleSelectMode()
    else if (e.code === 'KeyF' && !e.shiftKey) fit()
    else if (e.code === 'KeyR' && !e.shiftKey) relayout()
    else if (e.code === 'Delete' && e.shiftKey && canClear) void clearMap()
    else return
    e.preventDefault()
  }

  // Full screen: the field covers the window (CSS), and where the browser
  // allows it the window itself goes full screen too, hiding its chrome.
  // Leaving the browser's full screen (its own Esc) leaves ours as well.
  let full = $state(false)
  let nativeFull = false
  function toggleFull() {
    full = !full
    if (full) {
      const el = document.documentElement
      if (!document.fullscreenElement && el.requestFullscreen) {
        el.requestFullscreen().then(
          () => (nativeFull = true),
          () => {},
        )
      }
    } else if (nativeFull) {
      nativeFull = false
      if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    }
  }
  function onFullscreenChange() {
    if (!document.fullscreenElement && nativeFull) {
      nativeFull = false
      full = false
    }
  }
  $effect(() => {
    if (!active && untrack(() => full)) toggleFull()
  })

  /** Collapses bulky attachment lists into one cluster node. */
  function prepare(g: { root?: any; nodes?: any; edges?: any }, open_: Set<string>) {
    const nodes = g.nodes.map((n: any) => ({ ...n, x: 0, y: 0, w: 0, h: 0 }))
    const edges = g.edges.map((e: any) => ({ ...e }))
    const byKey = new Map<string, any>(nodes.map((n: { key: any }): [string, any] => [n.key, n]))
    const owners = new Map<string, any[]>()
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
      if (keys.length < CLUSTER_MIN || open_.has(ck) || loose.has(owner)) continue
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
      nodes: nodes.filter((n: { key: unknown }) => !drop.has(n.key)),
      edges: edges.filter((e: { from: unknown; to: unknown }) => !drop.has(e.from) && !drop.has(e.to)),
    }
  }

  /** Positions the user has set by hand (dragged cards), kept across rebuilds.
   * A key like `cluster:<owner>` also remembers where a folded attachment
   * group sat, so an expanded block lands there and folds back to the same spot. */
  const saved = new Map()

  /** Where an attachment group stood folded before it was opened; it goes back there. */
  const origins = new Map()
  function resetPositions() {
    saved.clear()
    origins.clear()
  }

  let buildSeq = 0
  /** Render the cards hidden, measure them, lay out, reveal. */
  async function rebuild(g: { root: any }, open_: Set<string>) {
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
    if (layoutMode !== 'tree') for (const n of view.nodes) delete n.group
    if (layoutMode === 'tree') groups = layoutTree(attached, edges, { loose })
    else if (layoutMode === 'down') groups = layoutTreeDown(attached, edges)
    else if (layoutMode === 'radial') groups = layoutRadial(attached, edges)
    else if (layoutMode === 'layers') groups = layoutLayers(attached)
    else {
      layoutGraph(attached, edges, { sizeOf: (n: { w: any; h: any }) => ({ w: n.w, h: n.h }) })
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

  /** Cards the user took off the map (kept in this browser; nothing is deleted). */
  const HIDDEN_KEY = 'quests.graph.hidden'
  const loadHidden = () => {
    try {
      const raw = JSON.parse(localStorage.getItem(HIDDEN_KEY) || '[]')
      return new Set(Array.isArray(raw) ? raw.filter((k) => typeof k === 'string') : [])
    } catch {
      return new Set()
    }
  }
  let hidden = $state(loadHidden())
  const saveHidden = (next: Set<unknown>) => {
    hidden = next
    try {
      localStorage.setItem(HIDDEN_KEY, JSON.stringify([...next]))
    } catch {
      /* storage may be unavailable; hiding still works until reload */
    }
  }

  // Which quests the map shows (and offers in "add to the map"). Default: active only.
  const STATUS_KEY = 'quests.graph.statuses'
  const loadStatuses = () => {
    try {
      const raw = JSON.parse(localStorage.getItem(STATUS_KEY) || 'null')
      if (Array.isArray(raw)) {
        const ok = raw.filter((x) => QUEST_STATUSES.includes(x))
        if (ok.length) return new Set(ok)
      }
    } catch {
      /* fall through to the default */
    }
    return new Set(['active'])
  }
  let questStatuses = $state(loadStatuses())
  const toggleStatus = (st: any) => {
    const next = new Set(questStatuses)
    if (next.has(st)) next.delete(st)
    else next.add(st)
    if (!next.size) return // at least one status stays on
    questStatuses = next
    try {
      localStorage.setItem(STATUS_KEY, JSON.stringify([...next]))
    } catch {
      /* the filter still works until reload */
    }
  }
  let statusLabel = $derived(
    questStatuses.size === QUEST_STATUSES.length
      ? 'все'
      : questStatuses.size === 1
        ? QUEST_STATUS_LABELS[[...questStatuses][0]]
        : `${questStatuses.size} статуса`,
  )
  /** Quests the status filter keeps off the map right now (for the counter). */
  let filteredOut = $state(0)

  // Which kinds of things the map shows (the centre and draft things always stay).
  const ENTITY_TYPES = ['questline', 'quest', 'step', 'note', 'attachment']
  const ENTITY_LABELS: Record<string, string> = { questline: 'Квестлайны', quest: 'Квесты', step: 'Шаги', note: 'Заметки', attachment: 'Вложения' }
  const TYPES_KEY = 'quests.graph.types'
  const loadTypes = () => {
    try {
      const raw = JSON.parse(localStorage.getItem(TYPES_KEY) || 'null')
      if (Array.isArray(raw)) {
        const ok = raw.filter((x) => ENTITY_TYPES.includes(x))
        if (ok.length) return new Set(ok)
      }
    } catch {
      /* fall through to everything */
    }
    return new Set(ENTITY_TYPES)
  }
  let shownTypes = $state(loadTypes())
  const toggleType = (t: any) => {
    const next = new Set(shownTypes)
    if (next.has(t)) next.delete(t)
    else next.add(t)
    if (!next.size) return
    shownTypes = next
    try {
      localStorage.setItem(TYPES_KEY, JSON.stringify([...next]))
    } catch {
      /* the filter still works until reload */
    }
  }
  let typesLabel = $derived(shownTypes.size === ENTITY_TYPES.length ? 'все' : `${shownTypes.size} из ${ENTITY_TYPES.length}`)

  // Cards whose direct links were asked for ("Показать связи"): kept in this browser.
  const EXPAND_KEY = 'quests.graph.expand'
  const loadExpand = () => {
    try {
      const raw = JSON.parse(localStorage.getItem(EXPAND_KEY) || '[]')
      return Array.isArray(raw) ? raw.filter((k) => typeof k === 'string') : []
    } catch {
      return []
    }
  }
  let expandKeys = $state((loadExpand() as string[]))
  const setExpand = (next: any[]) => {
    expandKeys = next
    try {
      localStorage.setItem(EXPAND_KEY, JSON.stringify(next))
    } catch {
      /* works until reload */
    }
  }

  const ownExpand = (key: string) => expandKeys.filter((k) => k === key || k.startsWith(`${key}>`))

  /** A quest the status filter would drop again, so opening it shows nothing. */
  function statusOff(key: string) {
    if (!key.startsWith('quest:')) return false
    const q = quests.find((x) => x.id === Number(key.slice('quest:'.length)))
    return Boolean(q) && !questStatuses.has(q.status)
  }

  /** Cards the user took off the map, per neighbour still on it: key → type → keys.
   * They are links like any other not shown yet, so they count in the circles. */
  let hiddenAdj = $derived.by(() => {
    const out = new Map<string, Record<string, string[]>>()
    if (!hidden.size) return out
    const add = (from: string, other: string) => {
      if (!hidden.has(other) || hidden.has(from)) return
      const type = other.slice(0, other.indexOf(':'))
      if (!out.has(from)) out.set(from, {})
      const by = out.get(from)
      if (!by[type]) by[type] = []
      if (!by[type].includes(other)) by[type].push(other)
    }
    for (const e of data?.edges ?? []) {
      add(e.from, e.to)
      add(e.to, e.from)
    }
    return out
  })

  /** Unopened links of a card by entity type, as circles on its border: what the
   * map has not reached yet plus what was taken off it. Kinds filtered out and
   * quests outside the status filter are not counted — opening them shows nothing. */
  function moreOf(n: { draft: any; more: {}; key: any }) {
    if (n.draft) return []
    const own: Record<string, string[]> = n.more ?? {}
    const gone: Record<string, string[]> = hiddenAdj.get(n.key) ?? {}
    const out = []
    for (const type of ENTITY_TYPES) {
      if (!shownTypes.has(type)) continue
      const keys = [...(own[type] ?? []), ...(gone[type] ?? [])]
      const count = keys.filter((k) => !statusOff(k)).length
      if (count) out.push({ type, count })
    }
    return out
  }

  /** Open only the links of one type of a card. */
  function openMore(n: { key: string; more: { [x: string]: string|any[] } }, type: string|number) {
    const back = hiddenAdj.get(n.key)?.[type]
    if (back?.length) {
      const next = new Set(hidden)
      for (const k of back) next.delete(k)
      saveHidden(next)
    }
    if (n.more?.[type]?.length) {
      const key = `${n.key}>${type}`
      if (!expandKeys.includes(key) && !expandKeys.includes(n.key)) setExpand([...expandKeys, key])
    }
  }

  // --- Selection mode: a marquee or clicks pick cards; the group moves together or leaves the map. ---
  let selectMode = $state(false)
  let picked: Set<string> = $state(new Set())
  let marquee = $state((null as { x0: number, y0: number, x1: number, y1: number } | null))

  function toggleSelectMode() {
    selectMode = !selectMode
    if (!selectMode) picked = new Set()
    marquee = null
    ctx = { ...ctx, open: false }
  }

  /** Takes the picked cards off the map. */
  async function hidePicked() {
    const keys = [...picked]
    picked = new Set()
    await hideKeys(keys)
  }

  /**
   * Takes cards off the map. Only what stays linked to a remaining card is
   * remembered as "hidden" (so it can be brought back); a card with no link
   * left to anything that stays simply goes. If nothing is left, the map is
   * cleared. Draft cards are not hidden (drop them from the log instead).
   */
  async function hideKeys(keys: string[]) {
    const gone = new Set(keys)
    const shown = view?.nodes ?? []
    const left = shown.filter((n) => !gone.has(n.key) && !n.draft)
    // Everything real is being removed, the centre included: start over.
    if (gone.has(data?.root) && !left.length) {
      await clearMap()
      return
    }
    const stays = new Set(shown.filter((n) => !gone.has(n.key)).map((n) => n.key))
    const linked = new Set()
    for (const e of view?.edges ?? []) {
      if (gone.has(e.from) && stays.has(e.to)) linked.add(e.from)
      if (gone.has(e.to) && stays.has(e.from)) linked.add(e.to)
    }
    const hide = new Set(hidden)
    const roots = [...draft.roots]
    let rootsChanged = false
    let skipped = 0
    for (const k of gone) {
      const n = nodeMap.get(k)
      if (!n || k === data?.root) continue
      if (n.draft) {
        skipped += 1
        continue
      }
      const at = roots.findIndex((r) => `${r.type}:${r.id}` === k)
      if (at >= 0) {
        roots.splice(at, 1)
        rootsChanged = true
      } else if (linked.has(k)) hide.add(k)
    }
    saveHidden(hide)
    if (rootsChanged) {
      try {
        await putGraphDraftRoots(roots)
        await loadDraft()
      } catch {
        /* toast already shown */
      }
    }
    if (skipped) toast('Карточки черновика не скрываются: уберите их из журнала', { kind: 'info' })
  }

  /** New steps / files with no owner: drawn red, they block saving. */
  let ownerless = $state((new Set() as Set<string>))
  /** Saved things that the draft writes into: node key → ops, oldest first. */
  let touched = $state((new Map() as Map<string, number[]>))
  /** The log row shown briefly after clicking a card's change marker. */
  let flashOp = $state((null as number | null))
  let flashTimer: number = 0

  /** Opens the log on the last change that wrote into this card. */
  function showChanges(key: string) {
    const ops = touched.get(key)
    if (!ops?.length) return
    draftOpen = true
    flashOp = ops[ops.length - 1]
    clearTimeout(flashTimer)
    flashTimer = window.setTimeout((): void => {
          flashOp = null
        }, 2800)
    void tick().then(() => {
      document.querySelector(`.gdraft__list [data-op="${flashOp}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  }

  let buildTick = $state(0)
  let buildLater = false
  $effect(() => {
    if (active && buildLater) {
      buildLater = false
      buildTick += 1
    }
  })

  // Rebuild when the data, the draft, or the set of expanded groups changes.
  $effect(() => {
    const g = data
    const open_ = expanded
    void loose
    const ops = draft.ops
    if (!g) return
    const gone = hidden
    const over = applyDraft(g, ops)
    ownerless = over.ownerless
    touched = over.touched
    // Hidden cards leave the map together with their edges (never the centre).
    const statuses = questStatuses
    const rootKeys = new Set([g.root, ...draft.roots.map((r) => `${r.type}:${r.id}`)])
    let nodes = over.nodes.filter((n) => !gone.has(n.key) || n.key === g.root)
    // Quests outside the chosen statuses leave the map, with the steps and
    // files that hang only off them. Centres and draft things always stay.
    const offQuests = new Set(
      nodes.filter((n) => n.type === 'quest' && !n.draft && !rootKeys.has(n.key) && !statuses.has(n.status)).map((n) => n.key),
    )
    if (offQuests.size) {
      const withThem = new Set(offQuests)
      for (const e of over.edges) {
        if (e.kind === 'structure' && offQuests.has(e.from) && /^(step|attachment):/.test(e.to)) withThem.add(e.to)
      }
      nodes = nodes.filter((n) => !withThem.has(n.key))
    }
    filteredOut = offQuests.size
    // What lost its link to every centre (hidden branches, filtered quests)
    // goes too; draft things stay wherever they are.
    {
      const adj = new Map()
      const link = (a: any, b: any) => {
        if (!adj.has(a)) adj.set(a, [])
        adj.get(a).push(b)
      }
      const present = new Set(nodes.map((n) => n.key))
      for (const e of over.edges) {
        if (present.has(e.from) && present.has(e.to)) {
          link(e.from, e.to)
          link(e.to, e.from)
        }
      }
      const reach = new Set([...rootKeys].filter((k) => present.has(k)))
      for (const k of reach) for (const nb of adj.get(k) ?? []) reach.add(nb)
      if (reach.size) nodes = nodes.filter((n) => n.draft || reach.has(n.key))
    }
    // Kinds switched off in the filter go last, so what is reached only through them stays.
    const kinds = shownTypes
    if (kinds.size < ENTITY_TYPES.length) {
      const off = new Set(
        nodes.filter((n) => !(n.draft || n.key === g.root || kinds.has(n.type) || !ENTITY_TYPES.includes(n.type))).map((n) => n.key),
      )
      // Steps and files go with the quest / note they belong to.
      const withThem = new Set(off)
      for (const e of over.edges) {
        if (e.kind === 'structure' && off.has(e.from) && /^(step|attachment):/.test(e.to)) withThem.add(e.to)
      }
      nodes = nodes.filter((n) => n.draft || n.key === g.root || !withThem.has(n.key))
    }
    const keep = new Set(nodes.map((n) => n.key))
    const edges = over.edges.filter((e) => keep.has(e.from) && keep.has(e.to))
    // Hidden page: cards cannot be measured; build when it is shown again.
    void buildTick
    if (!untrack(() => active)) {
      buildLater = true
      return
    }
    void rebuild({ ...g, nodes, edges }, open_)
  })

  /** Puts hand-placed cards back, and an expanded block where its cluster was. */
  function restoreSaved(groups: any[]) {
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
  const frameOf = (ms: any[]) => ({
    x0: Math.min(...ms.map((m: { x: number; w: number }) => m.x - m.w / 2)) - FRAME.left,
    x1: Math.max(...ms.map((m: { x: number; w: number }) => m.x + m.w / 2)) + FRAME.right,
    y0: Math.min(...ms.map((m: { y: number; h: number }) => m.y - m.h / 2)) - FRAME.top,
    y1: Math.max(...ms.map((m: { y: number; h: number }) => m.y + m.h / 2)) + FRAME.bottom,
  })

  /** Smallest nudge (down first, then sideways/up) that puts a block's frame
   * clear of every other card and group frame. {0,0} if it is already clear
   * or nothing nearby is free. */
  function freeShift(g: { key: any }, members: any[], allGroups: any) {
    if (!view) return { dx: 0, dy: 0 }
    const mine = new Set(members.map((m: { key: any }) => m.key))
    const gap = 18
    const obstacles: { x0: number; x1: any; y0: number; y1: any }[] = []
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
    const clear = (dx: number, dy: number) =>
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
    resetPositions()
    applyLayout()
    void tick().then(fit)
  }

  // --- Minimap: every card as a small rectangle, the visible part as a frame;
  // click or drag on it to move the view. ---
  const MINI_W = 170
  const MINI_H = 120
  let stageSize = $state({ w: 0, h: 0 })
  $effect(() => {
    const el = stageEl
    if (!el) return
    const ro = new ResizeObserver(() => {
      const w = el.clientWidth
      const h = el.clientHeight
      // Keep the middle of the view in place when the field grows or shrinks
      // (full screen, window resize); a hidden page (size 0) changes nothing.
      const prev = untrack(() => stageSize)
      if (prev.w && prev.h && w && h) {
        const c = untrack(() => cam)
        cam = { ...c, x: c.x + (w - prev.w) / 2, y: c.y + (h - prev.h) / 2 }
      }
      if (w && h) stageSize = { w, h }
    })
    ro.observe(el)
    return () => ro.disconnect()
  })
  // How much of a card is built right now, by its share of the map area on screen:
  // 'lite' — frame only (< 0.5%, or well outside the window); 'title' — the head
  // without the markdown body (0.5–1%); full (not in the map) — everything.
  // Cards keep their box, so nothing moves; the body is simply not built until
  // it matters. Each threshold has ±5% slack around the card's current tier, so
  // a card sitting right on a border does not flicker while zooming.
  const LITE_AREA = 0.005
  const FULL_AREA = 0.01
  const SLACK = 1.05
  let prevTier = new Map()
  let tier = $derived.by(() => {
    const next = new Map()
    const nodes = view?.nodes
    // Read everything up front so the dependencies are the same on every run.
    const { k, x: cx, y: cy } = cam
    const editingKey = editing?.key
    if (ready && nodes && stageSize.w && stageSize.h) {
      const W = stageSize.w
      const H = stageSize.h
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
        const was = prevTier.get(n.key) ?? 'full'
        const lite = share < (was === 'lite' ? LITE_AREA * SLACK : LITE_AREA / SLACK)
        const full = share >= (was === 'full' ? FULL_AREA / SLACK : FULL_AREA * SLACK)
        if (lite) next.set(n.key, 'lite')
        else if (!full) next.set(n.key, 'title')
      }
    }
    if (next.size === prevTier.size && [...next].every(([key, t]) => prevTier.get(key) === t)) return prevTier
    prevTier = next
    return next
  })

  /** Circles per card, worked out once per data/filter change, not per frame. */
  let moreMap = $derived.by(() => {
    const m = new Map()
    for (const n of data?.nodes ?? []) {
      const r = moreOf(n)
      if (r.length) m.set(n.key, r)
    }
    return m
  })

  let mini = $derived.by(() => {
    const nodes = (view?.nodes ?? []).filter((n) => ready && Number.isFinite(n.x) && n.w)
    if (!nodes.length || !stageSize.w) return null
    const vx = -cam.x / cam.k
    const vy = -cam.y / cam.k
    const vw = stageSize.w / cam.k
    const vh = stageSize.h / cam.k
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
  })
  let miniDrag = false
  /** Centres the view on the world point under the pointer on the minimap. */
  function miniGo(e: { currentTarget: HTMLElement; clientX: number; clientY: number }) {
    const m = mini
    if (!m) return
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const wx = (e.clientX - r.left) / m.sc + m.x0
    const wy = (e.clientY - r.top) / m.sc + m.y0
    cam = { ...cam, x: stageSize.w / 2 - wx * cam.k, y: stageSize.h / 2 - wy * cam.k }
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
  function sidesOf(a: { x: any; w: any; y: any; h?: number }, b: { x: any; w: any; y: any; h?: number }) {
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
  function edgePath(a: { x: number; w: number; y: number; h: number }, b: { x: number; w: number; y: number; h: number }, oa = 0, ob = 0) {
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
   */
  function spreadEnds(items: { e: any, a: any, b: any }[]) {
    const groups: Map<string, { item: any, end: 'a' | 'b', other: number }[]> = new Map()
    const push = (key: string, item: { e: any; a: any; b: any }, end: 'a' | 'b', other: any) => {
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
  const collapseKey = (g: { type: any; owner: any }) =>
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
    const items: { e: any, a: any, b: any, oa?: number, ob?: number }[] = []
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
  let drag: { moved?: any; kind: any; n?: any; nx?: any; ny?: any; items?: any; members?: any; k?: any; d?: any; wx?: any; wy?: any; x?: any; y?: any; cx?: any; cy?: any; additive?: any; key?: any } = null
  const MIN_K = 0.2
  const MAX_K = 2.5
  /** Limits a zoom step that starts at `from`. Fit can go below MIN_K on a big
   * map; from there zooming in must not jump straight to MIN_K. */
  const clampK = (k: number, from: number) => Math.max(Math.min(MIN_K, from), Math.min(Math.max(MAX_K, from), k))
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
    view = view && { ...view }
  }
  function pinchStart() {
    const [a, b] = [...touches.values()]
    const r = stageEl?.getBoundingClientRect()
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
  function onTouchDown(e: { pointerType: string; pointerId: number; clientX: any; clientY: any; stopPropagation: () => void; preventDefault: () => void }) {
    if (e.pointerType !== 'touch') return
    touches.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (touches.size !== 2) return
    e.stopPropagation()
    e.preventDefault()
    undoDrag()
    marquee = null
    pinchStart()
    stageEl?.setPointerCapture(e.pointerId)
  }
  function onBgDown(e: PointerEvent) {
    if ((e.target as Element).closest('[data-gkey]')) return
    // Middle button always pans, also in selection mode.
    if (e.button === 1) {
      e.preventDefault()
      drag = { kind: 'pan', x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y }
      stageEl?.setPointerCapture(e.pointerId)
      return
    }
    if (e.button !== 0) return
    if (selectMode) {
      const r = stageEl?.getBoundingClientRect()
      if (!r) return
      const x = e.clientX - r.left
      const y = e.clientY - r.top
      drag = { kind: 'marquee', additive: e.shiftKey || e.ctrlKey || e.metaKey }
      marquee = { x0: x, y0: y, x1: x, y1: y }
      stageEl?.setPointerCapture(e.pointerId)
      return
    }
    drag = { kind: 'pan', x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y }
    stageEl?.setPointerCapture(e.pointerId)
  }
  function onCardDown(e: PointerEvent, n: { key: any; x?: any; y?: any }) {
    if (linking) {
      if (e.button === 0) {
        e.stopPropagation()
        void finishLink(n)
      }
      return
    }
    if (selectMode) {
      if (e.button !== 0 || (e.target as Element).closest('textarea, input, .gcard__editbar, .gcard__changed')) return
      e.stopPropagation()
      const group = picked.has(n.key)
        ? [...picked].map((k) => nodeMap.get(k)).filter(Boolean)
        : [n]
      drag = {
        kind: 'multi',
        key: n.key,
        x: e.clientX,
        y: e.clientY,
        moved: false,
        items: group.map((m) => ({ m, x: m.x, y: m.y })),
      }
      stageEl?.setPointerCapture(e.pointerId)
      return
    }
    if (e.button !== 0) return
    if ((e.target as Element).closest('textarea, input, .gcard__editbar, .gcard__body a, .gcard__body button, .gcard__body pre')) return
    const body = (e.target as Element).closest('.gcard__body')
    // Inside the text area: let selection/scrolling happen; only the frame drags.
    if (body && body.scrollHeight > body.clientHeight + 1 && e.offsetX > body.clientWidth - 14) return
    e.stopPropagation()
    drag = { kind: 'node', n, x: e.clientX, y: e.clientY, nx: n.x, ny: n.y, moved: false }
    stageEl?.setPointerCapture(e.pointerId)
  }
  /** Dragging an expanded block by its frame moves all its cards together. */
  function onGroupDown(e: { button: number; stopPropagation: () => void; clientX: any; clientY: any; pointerId: number }, g: { key: any }) {
    if (e.button !== 0 || linking || selectMode) return
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
  function onMove(e: { pointerId?: any; clientX: any; clientY: any; preventDefault?: (() => void)|(() => void)|(() => void); stopPropagation?: (() => void)|(() => void)|(() => void) }) {
    if (touches.has(e.pointerId)) touches.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (drag?.kind === 'pinch') {
      if (touches.size < 2) return
      const [a, b] = [...touches.values()]
      const r = stageEl?.getBoundingClientRect()
      if (!r) return
      const k = clampK((drag.k * Math.hypot(a.x - b.x, a.y - b.y)) / drag.d, drag.k)
      cam = {
        k,
        x: (a.x + b.x) / 2 - r.left - drag.wx * k,
        y: (a.y + b.y) / 2 - r.top - drag.wy * k,
      }
      return
    }
    if (linking) linkCursor = worldAt(e)
    if (!drag) return
    if (drag.kind === 'marquee') {
      const r = stageEl?.getBoundingClientRect()
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
      view = view && { ...view }
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
      view = view && { ...view }
    } else {
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (!drag.moved) return
      drag.n.x = drag.nx + dx / cam.k
      drag.n.y = drag.ny + dy / cam.k
      view = view && { ...view }
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
        if (!drag.additive) picked = new Set()
      } else {
        const x0 = (Math.min(m.x0, m.x1) - cam.x) / cam.k
        const x1 = (Math.max(m.x0, m.x1) - cam.x) / cam.k
        const y0 = (Math.min(m.y0, m.y1) - cam.y) / cam.k
        const y1 = (Math.max(m.y0, m.y1) - cam.y) / cam.k
        const next: Set<string> = drag.additive ? new Set(picked) : new Set()
        for (const n of view?.nodes ?? []) {
          if (n.x + n.w / 2 >= x0 && n.x - n.w / 2 <= x1 && n.y + n.h / 2 >= y0 && n.y - n.h / 2 <= y1) next.add(n.key)
        }
        picked = next
      }
    }
    if (drag?.kind === 'multi') {
      if (drag.moved) {
        for (const s of drag.items) saved.set(s.m.key, { x: s.m.x, y: s.m.y })
      } else {
        const next = new Set(picked)
        if (next.has(drag.key)) next.delete(drag.key)
        else next.add(drag.key)
        picked = next
      }
    }
    if (drag?.kind === 'group' && drag.moved) {
      for (const s of drag.members) saved.set(s.m.key, { x: s.m.x, y: s.m.y })
    }
    if (drag?.kind === 'node' && drag.moved) saved.set(drag.n.key, { x: drag.n.x, y: drag.n.y })
    if (drag?.kind === 'node' && !drag.moved) select(drag.n)
    drag = null
  }
  function onWheel(e: WheelEvent) {
    // Over a card's scrollable text the wheel scrolls the text, not the map.
    const body = e.target instanceof Element ? (e.target as Element).closest('.gcard__body, textarea') : null
    if (body && body.scrollHeight > body.clientHeight + 1) return
    e.preventDefault()
    const r = stageEl?.getBoundingClientRect()
    if (!r) return
    // A touchpad pinch arrives as ctrl+wheel with small, frequent deltas:
    // follow them proportionally instead of a fixed step per event (capped,
    // so a ctrl+mouse-wheel notch is no bigger than a plain one).
    const f = e.ctrlKey
      ? Math.exp(-Math.max(-11, Math.min(11, e.deltaY)) * 0.01)
      : e.deltaY < 0 ? 1.12 : 1 / 1.12
    zoomAt(cam.k * f, e.clientX - r.left, e.clientY - r.top)
  }

  /** Folds an expanded attachment group back into its cluster node. */
  /** Centre of a group's cards (not of its frame, which has a header strip). */
  function membersCenter(g: { key: any; x: any; y: any }) {
    const ms = (view?.nodes ?? []).filter((n) => n.group === g.key)
    if (!ms.length) return { x: g.x, y: g.y }
    return {
      x: (Math.min(...ms.map((m) => m.x - m.w / 2)) + Math.max(...ms.map((m) => m.x + m.w / 2))) / 2,
      y: (Math.min(...ms.map((m) => m.y - m.h / 2)) + Math.max(...ms.map((m) => m.y + m.h / 2))) / 2,
    }
  }

  /** Folding puts the group back where it stood before it was opened (the open
   * block may have been nudged aside to keep clear of other cards). */
  function foldSpot(key: string) {
    const g = groups.find((x) => x.type === 'attachment' && `cluster:${x.owner}` === key)
    const spot = origins.get(key) ?? (g ? membersCenter(g) : null)
    if (spot) saved.set(key, spot)
    origins.delete(key)
  }

  function collapse(key: string) {
    foldSpot(key)
    expanded = new Set([...expanded].filter((k) => k !== key))
  }

  function collapseAll() {
    for (const k of expanded) foldSpot(k)
    expanded = new Set()
    loose = new Set()
  }

  /** Lays a file group out as separate cards. */
  function disband(owner: any) {
    const key = `cluster:${owner}`
    if (expanded.has(key)) foldSpot(key)
    expanded = new Set([...expanded].filter((k) => k !== key))
    loose = new Set([...loose, owner])
  }

  function select(n: { type: string; key: string; x: any; y: any }) {
    if (n.type === 'cluster') {
      saved.set(n.key, { x: n.x, y: n.y })
      origins.set(n.key, { x: n.x, y: n.y })
      expanded = new Set([...expanded, n.key])
      return
    }
    selectedKey = n.key
  }

  /** Where "Открыть" leads: a step opens its quest. */
  function openTarget(n: { draft: any; type: string; id: any; key: any }) {
    if (!n || n.draft) return null
    if (n.type === 'quest' || n.type === 'note' || n.type === 'attachment') return [n.type, n.id]
    if (n.type === 'step') {
      const e = data?.edges.find((x: { to: any; kind: string }) => x.to === n.key && x.kind === 'structure')
      const q = e && data.nodes.find((m: { key: any }) => m.key === e.from)
      return q ? ['quest', q.id] : null
    }
    return null
  }

  function recenter(n: { type: string; draft: any; id: number }) {
    if (!CENTERABLE.has(n.type) || n.draft) return
    resetPositions()
    setExpand([])
    center = { type: n.type, id: n.id }
    selectedKey = null
    onCenter?.(n.type, n.id)
  }

  // Root picker: quests and notes by title.
  let rootQuery = $state('')
  let pickOpen = $state(false)
  let found = $derived(findEntities(rootQuery, { questlines, quests, notes, limit: 12 }))
  let matches = $derived(found.items)
  let rootParsed = $derived(found.parsed)
  function chooseRoot(m: { type: string; id: number }) {
    rootQuery = ''
    pickOpen = false
    resetPositions()
    setExpand([])
    center = { type: m.type, id: m.id }
    selectedKey = null
    onCenter?.(m.type, m.id)
  }

  // --- Link mode: a line follows the cursor from a card; click a target to
  // make the link, right-click or Esc to cancel. Also used for "attach to". ---
  let linking: { mode: 'to' | 'from' | 'attach', anchor: string } | null = $state(null)
  let linkCursor = $state({ x: 0, y: 0 })

  const REF_FROM = new Set(['quest', 'note', 'step', 'questline'])
  /** Things the map can be centred on. */
  const CENTERABLE = new Set(['quest', 'note', 'questline'])
  const REF_TO = new Set(['quest', 'note', 'step', 'attachment', 'questline'])

  /** Whether the card can be picked as the other end in the current link mode. */
  function canPick(n: { key: any; type?: any }) {
    if (!linking || !n || n.key === linking.anchor || n.type === 'cluster') return false
    const a = nodeMap.get(linking.anchor)
    if (!a) return false
    if (linking.mode === 'to') return REF_TO.has(n.type)
    if (linking.mode === 'from') return REF_FROM.has(n.type)
    // a note goes under another note, but never under its own descendant
    if (a.type === 'note') return n.type === 'note' && !noteDescendants(a.key).has(n.key)
    // attach: a file goes to a quest / note / questline, a step to a quest
    return a.type === 'step' ? n.type === 'quest' : n.type === 'quest' || n.type === 'note' || n.type === 'questline'
  }

  /** Keys of the notes below this one, over the structure edges on the map (draft included). */
  function noteDescendants(key: any) {
    const seen = new Set()
    const walk = (k: any) => {
      for (const e of view?.edges ?? []) {
        if (e.kind === 'structure' && e.from === k && e.draft !== 'remove' && e.to.startsWith('note:') && !seen.has(e.to)) {
          seen.add(e.to)
          walk(e.to)
        }
      }
    }
    walk(key)
    return seen
  }

  /** Cards of one kind that belong to this card (steps of a quest, quests of a questline), as shown. */
  const childKeys = (n: { key: any }, type: string) =>
    (view?.edges ?? [])
      .filter((e) => e.kind === 'structure' && e.from === n.key && e.to.startsWith(`${type}:`) && !e.to.includes(':t') && nodeMap.has(e.to))
      .map((e) => e.to)

  /** The note this one hangs under, on the map. */
  const parentNote = (n: { key: any }) =>
    (view?.edges ?? []).find((e) => e.kind === 'structure' && e.to === n.key && e.from.startsWith('note:') && e.draft !== 'remove')?.from ?? null

  function startLink(mode: 'to' | 'from' | 'attach', n: { key: any }) {
    const a = nodeMap.get(n.key)
    linking = { mode, anchor: n.key }
    linkCursor = a ? { x: a.x, y: a.y } : { x: 0, y: 0 }
    ctx = { ...ctx, open: false }
  }

  function cancelLink() {
    linking = null
  }

  /** A reference from a to b that would survive saving already exists. */
  const hasRef = (from: any, to: any) =>
    (view?.edges ?? []).some((e) => e.kind === 'reference' && e.from === from && e.to === to && e.draft !== 'remove')

  async function finishLink(target: { key: any }) {
    const l = linking
    if (!l || !canPick(target)) return
    linking = null
    if (l.mode === 'attach') {
      const owned = (view?.edges ?? []).some((e) => e.kind === 'structure' && e.from === target.key && e.to === l.anchor)
      if (owned) {
        toast('Уже прикреплено сюда', { kind: 'info' })
        return
      }
      if (nodeMap.get(l.anchor)?.type === 'note') {
        await addOp('set_parent', { id: l.anchor, parent: target.key, was: parentNote(nodeMap.get(l.anchor)) ?? '' })
        return
      }
      await addOp('attach', { item: l.anchor, owner: target.key })
      return
    }
    const from = l.mode === 'to' ? l.anchor : target.key
    const to = l.mode === 'to' ? target.key : l.anchor
    if (hasRef(from, to)) {
      toast('Такая ссылка уже есть', { kind: 'info' })
      return
    }
    await addOp('add_ref', { from, to })
  }

  /** Links written in this card's own text, as targets still visible on the map. */
  function outgoingRefs(n: { key: any }) {
    return (view?.edges ?? [])
      .filter((e) => e.kind === 'reference' && e.from === n.key && e.draft !== 'remove' && nodeMap.has(e.to))
      .map((e) => e.to)
  }

  async function unlinkRef(n: { key: any }, targetKey: string) {
    const e = (view?.edges ?? []).find(
      (x) => x.kind === 'reference' && x.from === n.key && x.to === targetKey && x.draft !== 'remove',
    )
    // A link made earlier in this draft is undone by dropping that step of the log.
    if (e?.draft === 'add' && e.op) {
      await askDrop({ id: e.op })
      return
    }
    await addOp('remove_ref', { from: n.key, to: targetKey })
  }

  // Right-click menus: on the empty field (create / upload / add to map) and on a card.
  let ctx: { open: boolean, x: number, y: number, node: any, at: { x: number, y: number }, group: boolean, cluster?: any, edge?: any } = $state({ open: false, x: 0, y: 0, node: null, at: { x: 0, y: 0 }, group: false })

  /** Screen point → world coordinates (where a new thing should land). */
  function worldAt(e: { preventDefault?: (() => void)|(() => void)|(() => void); stopPropagation?: (() => void)|(() => void)|(() => void); clientX: any; clientY: any }) {
    const r = stageEl?.getBoundingClientRect()
    if (!r) return { x: 0, y: 0 }
    return { x: (e.clientX - r.left - cam.x) / cam.k, y: (e.clientY - r.top - cam.y) / cam.k }
  }

  function onCtx(e: MouseEvent, n: any = null) {
    e.preventDefault()
    e.stopPropagation()
    if (linking) {
      cancelLink()
      return
    }
    // With a group picked, the menu is just what a group can do.
    if (selectMode && picked.size && (!n || picked.has(n.key))) {
      ctx = { open: true, x: e.clientX, y: e.clientY, node: null, at: worldAt(e), group: true, cluster: null, edge: null }
      return
    }
    if (n?.type === 'cluster') {
      const owner = n.key.slice('cluster:'.length)
      ctx = { open: true, x: e.clientX, y: e.clientY, node: null, at: worldAt(e), group: false, cluster: { owner, key: n.key, folded: true, node: n }, edge: null }
      return
    }
    const real = n && n.type !== 'cluster' ? n : null
    if (real) selectedKey = real.key
    ctx = { open: true, x: e.clientX, y: e.clientY, node: real, at: worldAt(e), group: false, cluster: null, edge: null }
  }

  /** Right-click on the frame of an opened file block. */
  function onGroupCtx(e: { preventDefault: () => void; stopPropagation: () => void; clientX: any; clientY: any }, g: { type: string; owner: any }) {
    e.preventDefault()
    e.stopPropagation()
    if (linking || g.type !== 'attachment') return
    ctx = { open: true, x: e.clientX, y: e.clientY, node: null, at: worldAt(e), group: false, cluster: { owner: g.owner, key: collapseKey(g), folded: false, node: null }, edge: null }
  }

  /** What can be done to a line itself (nothing for plain ownership of steps and files). */
  function edgeActions(e: { draft: string; from: string; to: string; kind: string }) {
    if (e.draft === 'remove' || !e.from || !e.to) return []
    if (e.kind === 'reference') return [{ id: 'edge-unref', label: 'Разорвать ссылку' }]
    if (e.kind === 'dependency' && e.from.startsWith('step:') && e.to.startsWith('step:')) {
      return [{ id: 'edge-nowait', label: 'Не ждать предыдущий шаг' }]
    }
    if (e.kind === 'structure' && e.from.startsWith('questline:') && e.to.startsWith('quest:') && !e.to.includes(':t')) {
      return [{ id: 'edge-unline', label: 'Вывести квест из квестлайна' }]
    }
    if (e.kind === 'structure' && e.from.startsWith('note:') && e.to.startsWith('note:')) {
      return [{ id: 'edge-unparent', label: 'Открепить заметку' }]
    }
    return []
  }

  function onEdgeCtx(ev: { preventDefault: () => void; stopPropagation: () => void; clientX: any; clientY: any }, e: any) {
    ev.preventDefault()
    ev.stopPropagation()
    if (linking) {
      cancelLink()
      return
    }
    ctx = { open: true, x: ev.clientX, y: ev.clientY, node: null, at: worldAt(ev), group: false, cluster: null, edge: e }
  }

  const isExtraRoot = (n: { draft: any; key: string }) => n && !n.draft && draft.roots.some((r) => `${r.type}:${r.id}` === n.key)

  let ctxItems = $derived.by(() => {
    const n = ctx.node
    if (ctx.group) return [{ id: 'hide-picked', label: `Убрать с карты (${picked.size})` }]
    if (ctx.edge) return edgeActions(ctx.edge)
    if (ctx.cluster) {
      return [
        ctx.cluster.folded ? { id: 'cl-open', label: 'Развернуть' } : ctx.cluster.key ? { id: 'cl-fold', label: 'Свернуть' } : null,
        { id: 'cl-disband', label: 'Расформировать' },
      ].filter((x) => x !== null)
    }
    if (!n) {
      return [
        {
          id: 'create',
          label: 'Создать',
          children: [
            { id: 'new-quest', label: 'Квест' },
            { id: 'new-note', label: 'Заметку' },
            { id: 'new-step', label: 'Шаг' },
            { id: 'new-questline', label: 'Квестлайн' },
          ],
        },
        { id: 'upload', label: 'Загрузить файл…' },
        { id: 'find', label: 'Добавить на карту…' },
        { id: 'sep-sel', sep: true },
        { id: 'select-mode', label: selectMode ? 'Выйти из режима выделения' : 'Режим выделения' },
      ]
    }
    const items = []
    if (!n.draft) {
      if (openTarget(n)) items.push({ id: 'open', label: 'Открыть' })
      if (CENTERABLE.has(n.type) && n.key !== data?.root) items.push({ id: 'center', label: 'Сделать центром' })
      if (n.key !== data?.root) items.push({ id: isExtraRoot(n) ? 'unroot' : 'hide', label: 'Убрать с карты' })
      else if ((view?.nodes ?? []).every((x) => x.key === n.key || x.draft)) items.push({ id: 'hide', label: 'Убрать с карты' })
    }
    // What can be changed, duplicated, or put into this card.
    const own = []
    if (n.type === 'quest' || n.type === 'step' || n.type === 'note' || (n.type === 'questline' && n.draft)) own.push({ id: 'edit', label: 'Изменить…' })
    if (!n.draft && ['quest', 'note', 'step', 'attachment', 'questline'].includes(n.type)) own.push({ id: 'copy', label: 'Копировать' })
    const add = []
    if (n.type === 'quest') add.push({ id: 'add-step', label: 'Шаг' })
    if (n.type === 'quest' || n.type === 'note') add.push({ id: 'add-file', label: 'Вложение…' })
    if (n.type === 'quest' || n.type === 'step' || n.type === 'questline') add.push({ id: 'add-note', label: 'Заметку' })
    if (n.type === 'questline') add.push({ id: 'add-quest-line', label: 'Квест' })
    if (!n.draft && (n.type === 'quest' || n.type === 'note')) add.push({ id: 'new-quest-ref', label: 'Квест со ссылкой сюда' })
    if (add.length) own.push({ id: 'add', label: 'Добавить', children: add })
    if (own.length) {
      if (items.length) items.push({ id: 'sep-own', sep: true })
      items.push(...own)
    }
    if (REF_FROM.has(n.type)) {
      if (items.length) items.push({ id: 'sep-link', sep: true })
      items.push({ id: 'link-to', label: 'Добавить ссылку на…' })
      items.push({ id: 'link-from', label: 'Отметить как ссылку для…' })
      const out = outgoingRefs(n)
      if (out.length) {
        items.push({
          id: 'unlink',
          label: 'Убрать ссылку…',
          children: out.map((k) => ({ id: `unlink:${k}`, label: `→ ${titleOf(k)}` })),
        })
      }
    }
    // Exploring: bring in what this card is linked to, or take its steps / quests off.
    if (!n.draft) {
      const explore = []
      if (ownExpand(n.key).length) explore.push({ id: 'collapse', label: 'Свернуть связи' })
      if (!expandKeys.includes(n.key)) explore.push({ id: 'expand', label: 'Показать все связи' })
      if (n.type === 'quest' && childKeys(n, 'step').length) explore.push({ id: 'drop-steps', label: 'Убрать шаги' })
      if (n.type === 'questline' && childKeys(n, 'quest').length) explore.push({ id: 'drop-quests', label: 'Убрать квесты' })
      if (items.length) items.push({ id: 'sep-explore', sep: true })
      items.push(...explore)
    }
    if (n.type === 'step') items.push({ id: 'attach', label: 'Прикрепить к квесту…' })
    if (n.type === 'attachment') items.push({ id: 'attach', label: 'Прикрепить к…' })
    if (n.type === 'note') {
      items.push({ id: 'attach', label: 'Прикрепить к заметке…' })
      const up = parentNote(n)
      if (up) items.push({ id: 'detach-note', label: `Открепить от «${titleOf(up)}»` })
    }
    if (n.draft) {
      items.push({ id: 'sep-drop', sep: true })
      items.push({ id: 'drop', label: 'Убрать из черновика', danger: true })
    }
    return items
  })

  // Modals started from the menus.
  let questModal = $state({ open: false, desc: '', at: { x: 0, y: 0 }, questlineId: (null as number | null) })
  let entityModal = $state({
    open: false,
    kind: ('note' as 'note' | 'step' | 'questline'),
    at: { x: 0, y: 0 },
    /** quest key a new step is made for */
    owner: '',
    /** key of a card that should link to the new note */
    linkFrom: '',
    /** a thing that exists only in the draft, edited in place */
    edit: (null as { opId: number, payload: any, title: string, description: string, heading: string } | null),
  })
  let editQuest = $state({ open: false, quest: (null as any | null) })
  let editStep = $state({ open: false, questId: (null as number | null), step: (null as any | null) })
  let fileOwner = ''
  let findModal = $state(false)
  let fileInput = $state((null as HTMLInputElement | null))
  let fileAt = { x: 0, y: 0 }

  async function onCtxSelect(id: string) {
    const n = ctx.node
    const at = ctx.at
    const plain: any = { owner: '', linkFrom: '', edit: null }
    if (id === 'hide-picked') await hidePicked()
    else if (id === 'select-mode') toggleSelectMode()
    else if (id === 'new-quest') questModal = { open: true, desc: '', at, questlineId: null }
    else if (id === 'new-quest-ref' && n) questModal = { open: true, desc: `${n.type}=${n.id}`, at, questlineId: null }
    else if (id === 'add-quest-line' && n) questModal = { open: true, desc: '', at, questlineId: n.id }
    else if (id === 'new-note') await createNoteHere(at, '')
    else if (id === 'new-questline') entityModal = { open: true, kind: 'questline', at, ...plain }
    else if (id === 'new-step') entityModal = { open: true, kind: 'step', at, ...plain }
    else if (id === 'add-step' && n) entityModal = { open: true, kind: 'step', at, ...plain, owner: n.key }
    else if (id === 'add-note' && n) await createNoteHere(at, n.key)
    else if (id === 'edit' && n) await startEdit(n)
    else if (id === 'copy' && n) await addOp('copy', { source: n.key, pos: at })
    else if (id === 'upload' || id === 'add-file') {
      fileAt = at
      fileOwner = id === 'add-file' && n ? n.key : ''
      fileInput?.click()
    } else if (id === 'find') findModal = true
    else if (id === 'link-to' && n) startLink('to', n)
    else if (id === 'link-from' && n) startLink('from', n)
    else if (id === 'attach' && n) startLink('attach', n)
    else if (id === 'cl-open' && ctx.cluster?.node) select(ctx.cluster.node)
    else if (id === 'cl-fold' && ctx.cluster?.key) collapse(ctx.cluster.key)
    else if (id === 'cl-disband' && ctx.cluster) disband(ctx.cluster.owner)
    else if (id === 'edge-unref' && ctx.edge) await unlinkRef({ key: ctx.edge.from }, ctx.edge.to)
    else if (id === 'edge-nowait' && ctx.edge) await addOp('edit_step', { id: ctx.edge.to, fields: { wait_previous: false } })
    else if (id === 'edge-unline' && ctx.edge) await addOp('edit_quest', { id: ctx.edge.to, fields: { questline_id: null } })
    else if (id === 'edge-unparent' && ctx.edge) await addOp('set_parent', { id: ctx.edge.to, parent: '', was: ctx.edge.from })
    else if (id === 'expand' && n) setExpand([...expandKeys.filter((k) => !k.startsWith(`${n.key}>`)), n.key])
    else if (id === 'collapse' && n) setExpand(expandKeys.filter((k) => k !== n.key && !k.startsWith(`${n.key}>`)))
    else if (id === 'drop-steps' && n) await hideKeys(childKeys(n, 'step'))
    else if (id === 'drop-quests' && n) await hideKeys(childKeys(n, 'quest'))
    else if (id === 'detach-note' && n) await addOp('set_parent', { id: n.key, parent: '', was: parentNote(n) ?? '' })
    else if (id.startsWith('unlink:') && n) await unlinkRef(n, id.slice('unlink:'.length))
    else if (id === 'drop' && n?.op) await askDrop({ id: n.op })
    else if (id === 'hide' && n) await hideKeys([n.key])
    else if (id === 'unroot' && n) {
      const roots = draft.roots.filter((r) => `${r.type}:${r.id}` !== n.key)
      await putGraphDraftRoots(roots)
      await loadDraft()
    } else if (id === 'open' && n) {
      const t = openTarget(n)
      if (t) onOpen?.(t[0], t[1])
    } else if (id === 'center' && n) recenter(n)
  }

  async function onFilesPicked(e: { currentTarget: { files: any; value: string } }) {
    const files = [...(e.currentTarget.files ?? [])]
    e.currentTarget.value = ''
    for (const [i, file] of files.entries()) {
      try {
        const op = await uploadGraphDraftFile(file, fileOwner)
        saved.set(`attachment:t${op.id}`, { x: fileAt.x + i * 110, y: fileAt.y })
      } catch {
        /* toast already shown */
      }
    }
    await loadDraft()
  }

  // --- Editing. A saved quest / step opens its usual form (the change goes to
  // the draft); a thing that exists only in the draft is edited in place; a
  // note is edited right on its card, in markdown. ---
  const sameVal = (a: string|number|Date, b: string|number|Date, key: string) => {
    if ((a == null || a === '') && (b == null || b === '')) return true
    if (key === 'description') return String(a ?? '').trim() === String(b ?? '').trim()
    if (key.endsWith('_at')) return new Date(a).getTime() === new Date(b).getTime()
    return JSON.stringify(a) === JSON.stringify(b)
  }
  const diffFields = (orig: Record<string, any>, payload: Record<string, any>, keys: string[]) => {
    const out: Record<string, any> = {}
    for (const k of keys) if (k in payload && !sameVal(orig[k], payload[k], k)) out[k] = payload[k]
    return out
  }
  const QUEST_KEYS = ['title', 'description', 'status', 'significance', 'pinned', 'automated', 'sort_order', 'category_id', 'questline_id', 'deadline_at', 'duration_seconds']
  const STEP_KEYS = ['title', 'description', 'progress_current', 'progress_total', 'check_command', 'check_interval_seconds', 'wait_previous', 'run_mode', 'run_on']

  async function startEdit(n: { type?: any; draft: any; op: any; title: any; id: any; key: any }) {
    if (n.type === 'note') return startEditNote(n)
    if (n.draft) {
      const op = draft.ops.find((o) => o.id === n.op)
      if (!op) return
      const f = op.payload?.fields ?? {}
      entityModal = {
        open: true,
        kind: 'step',
        at: ctx.at,
        owner: '',
        linkFrom: '',
        edit: {
          opId: op.id,
          payload: op.payload,
          title: f.title ?? n.title,
          description: f.description ?? '',
          heading: n.type === 'quest' ? 'Изменить квест' : n.type === 'questline' ? 'Изменить квестлайн' : 'Изменить шаг',
        },
      }
      return
    }
    try {
      if (n.type === 'quest') {
        editQuest = { open: true, quest: await getQuestFull(n.id) }
      } else if (n.type === 'step') {
        const owner = (view?.edges ?? []).find((e) => e.kind === 'structure' && e.to === n.key && e.from.startsWith('quest:'))
        if (!owner) return
        const qid = Number(owner.from.slice('quest:'.length))
        const full = await getQuestFull(qid)
        const step = (full.steps ?? []).find((x) => x.id === n.id)
        if (step) editStep = { open: true, questId: qid, step }
      }
    } catch {
      /* toast already shown */
    }
  }

  async function draftEditQuest(payload: Record<string, any>) {
    const orig = editQuest.quest
    if (!orig) return
    const fields = diffFields(orig, payload, QUEST_KEYS)
    const was = (orig.tags ?? []).map((t: { id: any }) => t.id).sort((a: number, b: number) => a - b)
    const now = [...(payload.tag_ids ?? [])].sort((a, b) => a - b)
    if (JSON.stringify(was) !== JSON.stringify(now)) fields.tag_ids = payload.tag_ids
    if (!Object.keys(fields).length) {
      toast('Ничего не изменилось', { kind: 'info' })
      return
    }
    await addOp('edit_quest', { id: `quest:${orig.id}`, fields })
  }

  async function draftEditStep(payload: Record<string,any>) {
    const orig = editStep.step
    if (!orig) return
    const fields = diffFields(orig, payload, STEP_KEYS)
    if (!Object.keys(fields).length) {
      toast('Ничего не изменилось', { kind: 'info' })
      return
    }
    await addOp('edit_step', { id: `step:${orig.id}`, fields })
  }

  /** Submit of the small title + description form (new note / step, or an in-place edit). */
  async function onEntitySubmit(fields: any) {
    const m = entityModal
    if (m.edit) {
      try {
        await putGraphDraftOp(m.edit.opId, { ...m.edit.payload, fields: { ...(m.edit.payload.fields ?? {}), ...fields } })
        await loadDraft()
      } catch {
        /* toast already shown */
      }
      return
    }
    if (m.kind === 'step') {
      await addOp('create_step', { fields, ...(m.owner ? { quest: m.owner } : {}), pos: m.at })
      return
    }
    if (m.kind === 'questline') {
      await addOp('create_questline', { fields, pos: m.at })
      return
    }
    const op = await addOp('create_note', { fields, pos: m.at })
    if (op && m.linkFrom) await addOp('add_ref', { from: m.linkFrom, to: `note:t${op.id}` })
  }

  const NEW_NOTE = 'Новая заметка'

  /** "Новая заметка", then "Новая заметка (2)", (3)… — the first free one. */
  function freshNoteTitle() {
    const taken = new Set([
      ...notes.map((x) => x.title),
      ...(view?.nodes ?? []).filter((x) => x.type === 'note').map((x) => x.title),
      ...draft.ops.map((o) => o.payload?.fields?.title),
    ])
    if (!taken.has(NEW_NOTE)) return NEW_NOTE
    let i = 2
    while (taken.has(`${NEW_NOTE} (${i})`)) i += 1
    return `${NEW_NOTE} (${i})`
  }

  /** A new note appears at the cursor with its title selected, ready to type over. */
  async function createNoteHere(at: { x: number; y: number }, linkFrom: string) {
    const title = freshNoteTitle()
    const op = await addOp('create_note', { fields: { title, description: '' }, pos: at })
    if (!op) return
    if (linkFrom) await addOp('add_ref', { from: linkFrom, to: `note:t${op.id}` })
    await startEditNote({ key: `note:t${op.id}`, draft: true, op: op.id, title }, true)
  }

  /** Note edited on its card: the full text is fetched first (the map shows only the start of long notes). */
  let editing = $state(
    (null as { key: string, title: string, text: string, orig: { title: string, description: string }, opId: number | null, payload?: any, fresh?: boolean } | null),
  )

  async function startEditNote(n: { key: any; draft: any; op: any; title: any; id?: any }, fresh = false) {
    if (editing?.key === n.key) return
    try {
      if (n.draft) {
        const op = draft.ops.find((o) => o.id === n.op)
        if (!op) return
        const f = op.payload?.fields ?? {}
        editing = {
          key: n.key,
          title: f.title ?? n.title,
          text: f.description ?? '',
          orig: { title: f.title ?? '', description: f.description ?? '' },
          opId: op.id,
          payload: op.payload,
          fresh,
        }
      } else {
        const full = await getNote(n.id)
        editing = {
          key: n.key,
          title: full.title ?? n.title,
          text: full.description ?? '',
          orig: { title: full.title ?? '', description: full.description ?? '' },
          opId: null,
        }
      }
    } catch {
      /* toast already shown */
    }
  }

  async function finishEdit(saveIt: any) {
    const e = editing
    editing = null
    if (!saveIt || !e) return
    const changed: Record<string, any> = {}
    if (e.title.trim() && e.title.trim() !== e.orig.title) changed.title = e.title.trim()
    if (e.text !== e.orig.description) changed.description = e.text
    if (!Object.keys(changed).length) return
    if (e.opId != null) {
      try {
        await putGraphDraftOp(e.opId, { ...e.payload, fields: { ...(e.payload.fields ?? {}), ...changed } })
        await loadDraft()
      } catch {
        /* toast already shown */
      }
      return
    }
    await addOp('edit_note', { id: e.key, fields: changed })
  }

  const autofocus = (node: { focus: () => void }) => {
    node.focus()
  }

  const autofocusIf = (node: { focus: () => void }, on: any) => {
    if (on) node.focus()
  }

  /** Title of a brand-new note: focused with the whole text selected. */
  const focusSelect = (node: HTMLInputElement | HTMLTextAreaElement) => {
    // The card is mounted while the map is still hidden for measuring, and a
    // hidden field cannot take focus — keep trying until it shows.
    let tries = 0
    const grab = () => {
      node.focus()
      node.select()
      if (document.activeElement !== node && node.isConnected && tries++ < 30) requestAnimationFrame(grab)
    }
    grab()
  }

  /** Empty map: no centre, no extra centres, nothing hidden, layout forgotten.
   * Pending draft changes stay in the log (they have their own "Отменить всё"). */
  async function clearMap() {
    center = { type: null, id: null }
    selectedKey = null
    expanded = new Set()
    loose = new Set()
    linking = null
    editing = null
    resetPositions()
    setExpand([])
    saveHidden(new Set())
    try {
      await putGraphDraftRoots([])
      await loadDraft()
    } catch {
      /* toast already shown */
    }
    onCenter?.(null, null)
  }

  /** There is something to clear: a centre, extra centres, or hidden cards. */
  let canClear = $derived(Boolean(center.id) || draft.roots.length > 0 || hidden.size > 0)

  async function addToMap(type: string, id: number) {
    // First thing on an empty map becomes its centre.
    if (!center.id) {
      resetPositions()
      center = { type, id }
      onCenter?.(type, id)
      return
    }
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

  const titleOf = (key: any) => nodeMap.get(key)?.title ?? key

  const isDone = (n: { status: string }) => n.status === 'completed' || n.status === 'done'
  const isOrphan = (n: { type: string; links: number }) => (n.type === 'note' || n.type === 'attachment') && n.links === 0
  const rasterOf = (n: { type: string; mime: any }) => n.type === 'attachment' && RASTER.has(String(n.mime || '').split(';')[0].trim())
</script>

{#snippet glyph(n: any, t: any)}
  {#if (n.type === 'quest' || n.type === 'note' || n.type === 'questline') && (n.icon || n.icon_url)}
    <span class="gcard__icon" style:--line-color={n.color || t.color}>
      <QuestlineIcon icon={n.icon || 'document'} iconUrl={n.icon_url || null} size="sm" />
    </span>
  {:else}
    <span class="gcard__glyph">{n.type === 'step' && isDone(n) ? '✓' : t.glyph}</span>
  {/if}
{/snippet}

<svelte:window
  onkeydown={onKey}
/>
<svelte:document onfullscreenchange={onFullscreenChange} />

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
        title={`Фильтры: ${helpLine(ENTITY_SPEC)}`}
      />
      <FlagHints value={rootQuery} spec={ENTITY_SPEC} parsed={rootParsed} onPick={(v) => (rootQuery = v)} />
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
      <select
        bind:value={layoutMode}
        onchange={() => {
          try {
            localStorage.setItem(LAYOUT_KEY, layoutMode)
          } catch {
            /* the choice just is not remembered */
          }
          relayout()
        }}
        aria-label="Раскладка"
      >
        {#each Object.entries(LAYOUTS) as [k, label] (k)}
          <option value={k}>{label}</option>
        {/each}
      </select>
    </label>
    <details class="graph__filter">
      <summary title="Какие квесты показывать на карте и предлагать в «Добавить на карту»">
        Квесты: {statusLabel}{#if filteredOut} · скрыто {filteredOut}{/if}
      </summary>
      <div class="graph__filter-pop">
        {#each QUEST_STATUSES as st (st)}
          <label>
            <input type="checkbox" checked={questStatuses.has(st)} onchange={() => toggleStatus(st)} />
            {QUEST_STATUS_LABELS[st]}
          </label>
        {/each}
      </div>
    </details>
    <details class="graph__filter">
      <summary title="Какие сущности показывать на карте">Сущности: {typesLabel}</summary>
      <div class="graph__filter-pop">
        {#each ENTITY_TYPES as t (t)}
          <label>
            <input type="checkbox" checked={shownTypes.has(t)} onchange={() => toggleType(t)} />
            {ENTITY_LABELS[t]}
          </label>
        {/each}
      </div>
    </details>
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
    {#if expanded.size || loose.size}
      <button type="button" class="graph__btn" onclick={collapseAll}>Свернуть вложения</button>
    {/if}
  </div>

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="graph__stage"
    class:graph__stage--full={full}
    data-swipe={view?.nodes?.length ? undefined : 'free'}
    bind:this={stageEl}
    onpointerdowncapture={onTouchDown}
    onpointerdown={onBgDown}
    onpointermove={onMove}
    onpointerup={onUp}
    onpointercancel={onUp}
    onwheel={onWheel}
    oncontextmenu={(e) => onCtx(e)}
  >
    {#if linking}
      <p class="glink-hint">
        {#if linking.mode === 'to'}
          Выберите, на что ссылаться
        {:else if linking.mode === 'from'}
          Выберите, где должна появиться ссылка на «{titleOf(linking.anchor)}»
        {:else}
          Выберите, к чему прикрепить «{titleOf(linking.anchor)}»
        {/if}
        · ПКМ или Esc — отмена
      </p>
    {/if}
    {#if !center.id && !view?.nodes?.length}
      <p class="graph__msg">Выберите квест или заметку в поиске выше — или откройте карту из карточки квеста. ПКМ по полю создаёт новые.</p>
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
            marker-end={`url(#arrow-${e.kind})`}
          />
          {#if edgeActions(e).length}
            <path d={e.d} fill="none" stroke="transparent" stroke-width="14" class="gedge-hit" oncontextmenu={(ev) => onEdgeCtx(ev, e)} />
          {/if}
          {#if e.kind !== 'structure'}
            <circle cx={e.sx} cy={e.sy} r="3.4" fill={e.draft === 'remove' ? '#d23c3c' : EDGES[e.kind].color} />
          {/if}
        {/each}
        {#if linking && nodeMap.get(linking.anchor)}
          {@const a = nodeMap.get(linking.anchor)}
          <line
            x1={a.x}
            y1={a.y}
            x2={linkCursor.x}
            y2={linkCursor.y}
            stroke={linking.mode === 'attach' ? EDGES.structure.color : EDGES.reference.color}
            stroke-width="2.4"
            stroke-dasharray="6 4"
            class="glink-line"
          />
          <circle cx={linkCursor.x} cy={linkCursor.y} r="4.5" fill={EDGES.reference.color} />
        {/if}
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
          oncontextmenu={(e) => onGroupCtx(e, g)}
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
          class:gcard--dim={linking && n.key !== linking.anchor && !canPick(n)}
          class:gcard--pickable={linking && canPick(n)}
          class:gcard--anchor={linking?.anchor === n.key}
          class:gcard--ownerless={ownerless.has(n.key)}
          class:gcard--editing={editing?.key === n.key}
          class:gcard--picked={picked.has(n.key)}
          class:gcard--pill={!big && !square && !withText}
          style:--c={ownerless.has(n.key) ? `color-mix(in srgb, ${t.color} 35%, #e0443e)` : t.color}
          style:left="{n.x - n.w / 2}px"
          style:top="{n.y - n.h / 2}px"
          style:width={big ? `${CARD[n.type].w}px` : withText ? `${CARD.stepBody.w}px` : square ? `${SQUARE}px` : undefined}
          style:height={ready && n.h ? `${editing?.key === n.key ? Math.max(n.h, 340) : n.h}px` : undefined}
          data-gkey={n.key}
          title="{t.label}: {n.title}{isOrphan(n) ? ' — на неё нет ссылок' : ''}"
          onpointerdown={(e) => onCardDown(e, n)}
          oncontextmenu={(e) => onCtx(e, n)}
          ondblclick={(e) => {
            if (n.type === 'note' && !(e.target instanceof Element && (e.target as Element).closest('a, button, textarea, input'))) void startEditNote(n)
          }}
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
          {:else if editing && editing.key === n.key}
            <div class="gcard__head">
              {@render glyph(n, t)}
              {#if editing.fresh}
                <input
                  class="gcard__titleedit"
                  bind:value={editing.title}
                  aria-label="Заголовок"
                  use:focusSelect
                  onkeydown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      e.currentTarget.closest('.gcard')?.querySelector('textarea')?.focus()
                    } else if (e.key === 'Escape') {
                      e.stopPropagation()
                      void finishEdit(true)
                    }
                  }}
                />
              {:else}
                <input class="gcard__titleedit" bind:value={editing.title} aria-label="Заголовок" />
              {/if}
            </div>
            <textarea
              class="gcard__editor"
              bind:value={editing.text}
              use:autofocusIf={!editing.fresh}
              placeholder="Markdown"
              spellcheck="false"
              onkeydown={(e) => {
                if (e.key === 'Escape') {
                  e.stopPropagation()
                  void finishEdit(false)
                } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                  e.preventDefault()
                  void finishEdit(true)
                }
              }}
            ></textarea>
            <div class="gcard__editbar">
              <button type="button" class="graph__btn graph__btn--primary" onclick={() => finishEdit(true)}>Готово</button>
              <button type="button" class="graph__btn" onclick={() => finishEdit(false)}>Отмена</button>
              <span class="gcard__edithint">Ctrl+Enter — готово · Esc — отмена</span>
            </div>
          {:else if tier.get(n.key) === 'lite'}
            <!-- frame only: off-screen or too small to read -->
          {:else if big || withText}
            <div class="gcard__head">
              {@render glyph(n, t)}
              <span class="gcard__title">{n.title}</span>
              {#if n.type === 'quest' && n.status && n.status !== 'active'}
                <span class="gcard__status">{n.status}</span>
              {:else if n.type === 'step' && n.progress}
                <span class="gcard__prog">{n.progress}</span>
              {/if}
            </div>
            {#if n.desc && tier.get(n.key) !== 'title'}
              <div class="gcard__body" style:max-height="{big ? CARD[n.type].max : CARD.stepBody.max}px">
                <MarkdownBody source={n.desc} labels={{}} onRef={(k, id) => onOpen?.(k, id)} />
              </div>
            {/if}
          {:else}
            {@render glyph(n, t)}
            <span class="gcard__title gcard__title--line">{n.title}</span>
            {#if n.type === 'step' && n.progress}
              <span class="gcard__prog">{n.progress}</span>
            {/if}
          {/if}
          {#if !square && tier.get(n.key) !== 'lite'}
            {@const more = moreMap.get(n.key) ?? []}
            {#if more.length}
              <div class="gcard__more">
                {#each more as m (m.type)}
                  <button
                    type="button"
                    class="gcard__dot"
                    style:--dot={TYPES[m.type].color}
                    title="{ENTITY_LABELS[m.type]}: {m.count} не на карте. Клик — показать"
                    aria-label="Показать связи: {ENTITY_LABELS[m.type]}, {m.count}"
                    onpointerdown={(e) => e.stopPropagation()}
                    oncontextmenu={(e) => e.stopPropagation()}
                    onclick={() => openMore(n, m.type)}>{m.count}</button
                  >
                {/each}
              </div>
            {/if}
          {/if}
          {#if touched.has(n.key) && !n.draft}
            <button
              type="button"
              class="gcard__changed"
              title="Есть несохранённые изменения — показать в журнале"
              aria-label="Есть несохранённые изменения"
              onpointerdown={(e) => e.stopPropagation()}
              onclick={() => showChanges(n.key)}>!</button
            >
          {/if}
        </div>
      {/each}
    </div>

    {#if marquee}
      <div
        class="gmarquee"
        style:left="{Math.min(marquee.x0, marquee.x1)}px"
        style:top="{Math.min(marquee.y0, marquee.y1)}px"
        style:width="{Math.abs(marquee.x1 - marquee.x0)}px"
        style:height="{Math.abs(marquee.y1 - marquee.y0)}px"
      ></div>
    {/if}

    {#if mini}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="gmini"
        style:width="{mini.w}px"
        style:height="{mini.h}px"
        title="Миникарта: клик или перетаскивание — переместить вид"
        onpointerdown={(e) => {
          e.stopPropagation()
          miniDrag = true
          e.currentTarget.setPointerCapture(e.pointerId)
          miniGo(e)
        }}
        onpointermove={(e) => miniDrag && miniGo(e)}
        onpointerup={() => (miniDrag = false)}
        onpointercancel={() => (miniDrag = false)}
        onwheel={(e) => e.stopPropagation()}
        oncontextmenu={(e) => {
          e.preventDefault()
          e.stopPropagation()
        }}
      >
        <svg width={mini.w} height={mini.h} aria-hidden="true">
          {#each mini.nodes as n (n.key)}
            <rect
              x={(n.x - n.w / 2 - mini.x0) * mini.sc}
              y={(n.y - n.h / 2 - mini.y0) * mini.sc}
              width={Math.max(2, n.w * mini.sc)}
              height={Math.max(2, n.h * mini.sc)}
              rx="1"
              fill={TYPES[n.type]?.color ?? '#888'}
              opacity={n.draft ? 1 : 0.7}
            />
          {/each}
          <rect
            class="gmini__view"
            x={(mini.view.x - mini.x0) * mini.sc}
            y={(mini.view.y - mini.y0) * mini.sc}
            width={mini.view.w * mini.sc}
            height={mini.view.h * mini.sc}
          />
        </svg>
      </div>
    {/if}

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="gtools"
      onpointerdown={(e) => e.stopPropagation()}
      onwheel={(e) => e.stopPropagation()}
      oncontextmenu={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
    >
      {#if selectMode && picked.size}
        <span class="gtools__count">Выделено: {picked.size}</span>
        <button type="button" class="gtool" title="Убрать выделенное с карты" aria-label="Убрать выделенное с карты" onclick={hidePicked}>
          <Icon name="eye-off" size={16} />
        </button>
        <button type="button" class="gtool" title="Снять выделение" aria-label="Снять выделение" onclick={() => (picked = new Set())}>
          <Icon name="close" size={16} />
        </button>
        <span class="gtools__sep" aria-hidden="true"></span>
      {/if}
      <button
        type="button"
        class="gtool"
        class:gtool--on={selectMode}
        aria-pressed={selectMode}
        title="Режим выделения (V): рамкой по полю или кликами по карточкам. Выделенные двигаются вместе. Esc — выйти"
        aria-label="Режим выделения"
        onclick={toggleSelectMode}
      >
        <Icon name="select" size={16} />
      </button>
      <button type="button" class="gtool" title="Вписать карту в окно (F)" aria-label="Вписать карту в окно" onclick={fit}>
        <Icon name="fit" size={16} />
      </button>
      <button type="button" class="gtool" title="Распутать: пересчитать раскладку (R)" aria-label="Распутать" onclick={relayout}>
        <Icon name="arrange" size={16} />
      </button>
      <button
        type="button"
        class="gtool"
        class:gtool--on={full}
        aria-pressed={full}
        title={full ? 'Выйти из полноэкранного режима (Esc)' : 'Полноэкранный режим'}
        aria-label="Полноэкранный режим"
        onclick={toggleFull}
      >
        <Icon name={full ? 'fullscreen-exit' : 'fullscreen'} size={16} />
      </button>
      <button
        type="button"
        class="gtool"
        disabled={!canClear}
        title="Очистить карту (Shift+Delete): центр, добавленные центры и скрытое. Журнал изменений остаётся"
        aria-label="Очистить карту"
        onclick={clearMap}
      >
        <Icon name="clear" size={16} />
      </button>
    </div>

    {#if draft.ops.length}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="gdraft"
        class:gdraft--strip={!draftOpen}
        onpointerdown={(e) => e.stopPropagation()}
        onwheel={(e) => e.stopPropagation()}
        oncontextmenu={(e) => {
          e.preventDefault()
          e.stopPropagation()
        }}
      >
        <div class="gdraft__head">
          <button
            type="button"
            class="gdraft__toggle"
            aria-expanded={draftOpen}
            title={draftOpen ? 'Свернуть в полоску' : 'Развернуть журнал'}
            onclick={() => (draftOpen = !draftOpen)}
          >
            <Icon name={draftOpen ? 'chevron-down' : 'chevron-right'} size={12} />
            Журнал ({draft.ops.length})
          </button>
          <span class="gdraft__btns">
            <button
              type="button"
              class="graph__btn graph__btn--primary"
              disabled={committing || ownerless.size > 0}
              title={ownerless.size ? 'Есть невалидные элементы (красная рамка)' : 'Применить все изменения (Ctrl+S)'}
              onclick={saveDraft}
            >
              {committing ? 'Сохраняю…' : 'Сохранить'}
            </button>
            <button type="button" class="graph__btn" onclick={() => (cancelAsk = true)}>Отменить всё</button>
          </span>
        </div>
        {#if draftOpen}
          <p class="gdraft__hint">
            {#if ownerless.size}
              Нужно исправить: {ownerless.size} (красная рамка) — шаг или файл без владельца, квест без шагов. Пока не исправлено, сохранить нельзя.
            {:else}
              В базе пока ничего нет — сохраните или отмените.
            {/if}
          </p>
          <ol class="gdraft__list">
            {#each draft.ops as op (op.id)}
              <li class:gdraft__row--bad={opProblems[op.id]} class:gdraft__row--flash={flashOp === op.id} data-op={op.id}>
                <span class="gdraft__text">{describeOp(op, titleOf)}</span>
                {#if opProblems[op.id]}<span class="gdraft__msg">{opProblems[op.id]}</span>{/if}
                <button type="button" class="gdraft__x" title="Убрать из черновика" aria-label="Убрать из черновика" onclick={() => askDrop(op)}>
                  <Icon name="close" size={12} />
                </button>
              </li>
            {/each}
          </ol>
        {/if}
      </div>
    {/if}

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
      {#if selected.key !== data?.root && CENTERABLE.has(selected.type) && !selected.draft}
        <button type="button" class="graph__btn" onclick={() => recenter(selected)}>Сделать центром</button>
      {/if}
    {:else}
      <span class="graph__meta">
        Клик по карточке — подробности, ПКМ — действия. Пунктирная рамка у заметки или вложения — на неё никто не ссылается.
        <span class="graph__keys">
          {#each ['quest', 'step', 'note', 'attachment', 'questline', 'template'] as k (k)}
            <span class="graph__key" style:color={TYPES[k].color}>{TYPES[k].glyph} {TYPES[k].label}</span>
          {/each}
        </span>
      </span>
    {/if}
  </div>
</div>

<input bind:this={fileInput} type="file" multiple hidden onchange={onFilesPicked} />

<QuestModal
  open={questModal.open}
  mode="create"
  defaults={{ description: questModal.desc, questline_id: questModal.questlineId }}
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
  initial={entityModal.edit ? { title: entityModal.edit.title, description: entityModal.edit.description } : null}
  heading={entityModal.edit?.heading ?? ''}
  {quests}
  {notes}
  onClose={() => (entityModal = { ...entityModal, open: false })}
  onSubmit={onEntitySubmit}
/>

<QuestModal
  open={editQuest.open}
  mode="edit"
  quest={editQuest.quest}
  {quests}
  {notes}
  attachments={[]}
  onClose={() => (editQuest = { ...editQuest, open: false })}
  onSaved={() => {}}
  onDraft={draftEditQuest}
/>

<StepModal
  open={editStep.open}
  questId={editStep.questId}
  step={editStep.step}
  {quests}
  {notes}
  questlines={[]}
  attachments={[]}
  onClose={() => (editStep = { ...editStep, open: false })}
  onSaved={() => {}}
  onDraft={draftEditStep}
/>

<!-- Quests follow the status filter; notes and questlines are offered as they are. -->
<FindEntityModal
  open={findModal}
  types={['quest', 'note', 'questline']}
  quests={quests.filter((q) => questStatuses.has(q.status))}
  {notes}
  {questlines}
  onClose={() => (findModal = false)}
  onPick={addToMap}
/>

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
    flex: 1 1 18rem;
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

  /* Above the journal chrome, below modals (50) and menus (80). */
  .graph__stage--full {
    position: fixed;
    inset: 0;
    z-index: 40;
    min-height: 0;
    border: none;
    border-radius: 0;
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
    min-width: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  /* Wide: in the line after the hint. Narrow: two rows of three. */
  .graph__keys {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 0 0.6rem;
    margin-left: 0.6rem;
  }

  .graph__key {
    white-space: nowrap;
  }

  @media (max-width: 720px) {
    .graph__keys {
      display: grid;
      grid-template-columns: repeat(3, auto);
      justify-content: start;
      gap: 0.15rem 0.9rem;
      margin: 0.3rem 0 0;
    }
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

  .gedge-hit {
    pointer-events: stroke;
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

  .gcard--editing {
    z-index: 5;
    cursor: default;
    box-shadow: 0 6px 24px color-mix(in srgb, #000 55%, transparent);
  }

  .gcard__titleedit {
    flex: 1 1 auto;
    min-width: 0;
    padding: 0.15rem 0.35rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 6px;
    background: color-mix(in srgb, var(--c, #888) 14%, var(--color-bg, #121212));
    color: inherit;
    font: inherit;
    font-weight: 600;
  }

  .gcard__editor {
    flex: 1 1 auto;
    min-height: 0;
    resize: none;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 6px;
    background: color-mix(in srgb, var(--c, #888) 14%, var(--color-bg, #121212));
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-mono, monospace);
    font-size: 0.78rem;
    line-height: 1.45;
    user-select: text;
  }

  .gcard__editbar {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .gcard__edithint {
    margin-left: auto;
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
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

  .gcard--picked {
    outline: 3px solid #5aa9ff;
    outline-offset: 2px;
  }

  .gmarquee {
    position: absolute;
    z-index: 7;
    border: 1px solid #5aa9ff;
    background: color-mix(in srgb, #5aa9ff 14%, transparent);
    pointer-events: none;
  }

  /* Map tools: flat square icon buttons in the bottom-right corner. */
  .gmini {
    position: absolute;
    z-index: 6;
    left: 0.6rem;
    top: 0.6rem;
    box-shadow: 0 0 0 1px var(--color-border, #333);
    border-radius: 3px;
    background: color-mix(in srgb, var(--pill-bg, #0f0f0f) 85%, transparent);
    cursor: pointer;
    touch-action: none;
  }

  .gmini svg {
    display: block;
  }

  .gmini__view {
    fill: rgba(201, 162, 39, 0.05);
    stroke: var(--color-accent, #c9a227);
    stroke-width: 1.2;
  }

  .gtools {
    position: absolute;
    z-index: 6;
    right: 0.6rem;
    bottom: 0.6rem;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    cursor: default;
  }

  .gtool {
    display: grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    padding: 0;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .gtool:hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
    border-color: var(--color-fg-subtle, #6e6e6e);
  }

  .gtool:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .gtool--on {
    color: #5aa9ff;
    border-color: #5aa9ff;
  }

  .gtools__count {
    margin-right: 0.15rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gtools__sep {
    width: 1px;
    height: 1.2rem;
    margin: 0 0.1rem;
    background: var(--color-border, #333);
  }

  /* The folded log is a capsule; its buttons are capsules too. */
  .gdraft--strip .graph__btn {
    border-radius: 999px;
    padding-inline: 0.8rem;
  }

  .graph__btn--on {
    border-color: #5aa9ff;
    color: #5aa9ff;
  }

  .graph__filter {
    position: relative;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__filter summary {
    cursor: pointer;
    padding: 0.25rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    color: var(--color-fg, #e8e8e8);
    list-style: none;
  }

  .graph__filter summary::-webkit-details-marker {
    display: none;
  }

  .graph__filter-pop {
    position: absolute;
    z-index: 8;
    top: calc(100% + 0.3rem);
    left: 0;
    display: grid;
    gap: 0.25rem;
    min-width: 11rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: var(--pill-shadow, none);
  }

  .graph__filter-pop label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
  }

  /* Link mode: what can be picked glows, the rest recedes. */
  .gcard--dim {
    opacity: 0.35;
  }

  .gcard--pickable {
    cursor: crosshair;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c, #c9a227) 35%, transparent);
  }

  .gcard--pickable:hover {
    box-shadow: 0 0 0 4px var(--c, #c9a227);
  }

  .gcard--anchor {
    box-shadow: 0 0 0 4px var(--c, #c9a227);
  }

  .glink-hint {
    position: absolute;
    z-index: 4;
    top: 0.6rem;
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 0.35rem 0.9rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: 999px;
    background: var(--color-bg-raised, #1a1a1a);
    font-size: var(--text-sm, 0.875rem);
    pointer-events: none;
  }

  .glink-line {
    pointer-events: none;
  }

  /* New, not yet saved: dotted frame; without an owner the frame turns red. */
  .gcard--draft {
    border-style: dotted;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c, #888) 14%, transparent);
  }

  .gcard--ownerless {
    box-shadow: 0 0 0 3px color-mix(in srgb, #e0443e 22%, transparent);
  }

  /* The change log floats over the top-right of the field; folded it is a strip. */
  .gdraft {
    position: absolute;
    z-index: 6;
    top: 0.6rem;
    right: 0.6rem;
    width: min(24rem, calc(100% - 1.2rem));
    max-height: calc(100% - 1.2rem);
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid color-mix(in srgb, var(--color-accent, #c9a227) 60%, transparent);
    border-radius: 12px;
    background: color-mix(in srgb, var(--color-accent, #c9a227) 9%, var(--color-bg-raised, #1a1a1a));
    box-shadow: 0 4px 18px color-mix(in srgb, #000 45%, transparent);
    font-size: var(--text-sm, 0.875rem);
    cursor: default;
  }

  .gdraft--strip {
    width: auto;
    max-width: calc(100% - 1.2rem);
    padding: 0.3rem 0.5rem;
    border-radius: 999px;
  }

  .gdraft__head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .gdraft__btns {
    display: inline-flex;
    gap: 0.4rem;
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
    margin: 0;
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
    min-height: 0;
    overflow: auto;
    border: 1px solid var(--color-border, #333);
    border-radius: 10px;
    background: color-mix(in srgb, #000 18%, transparent);
    font-size: var(--text-xs, 0.75rem);
  }

  .gdraft__list li {
    display: flex;
    align-items: center;
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
    display: inline-grid;
    place-items: center;
    width: 1.3rem;
    height: 1.3rem;
    padding: 0;
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

  .gcard__icon {
    flex: none;
    display: inline-flex;
    align-self: center;
    color: var(--line-color, var(--c, #888));
  }

  /* Links not on the map yet: a circle per entity type on the card's lower border. */
  .gcard__more {
    position: absolute;
    left: 0;
    right: 0;
    bottom: -11px;
    z-index: 2;
    display: flex;
    justify-content: center;
    gap: 4px;
    pointer-events: none;
  }

  .gcard__dot {
    min-width: 21px;
    height: 21px;
    padding: 0 5px;
    box-sizing: border-box;
    border: 2px solid var(--color-bg, #121212);
    border-radius: 999px;
    background: var(--dot, #888);
    color: #141414;
    font: inherit;
    font-size: 11px;
    font-weight: 700;
    line-height: 17px;
    text-align: center;
    cursor: pointer;
    pointer-events: auto;
    box-shadow: 0 1px 4px color-mix(in srgb, #000 50%, transparent);
  }

  .gcard__dot:hover {
    filter: brightness(1.15);
    transform: scale(1.12);
  }

  /* Unsaved change: a yellow "!" in a circle on the card's top-right corner. */
  .gcard__changed {
    position: absolute;
    top: -9px;
    right: -9px;
    z-index: 2;
    width: 19px;
    height: 19px;
    padding: 0;
    display: grid;
    place-items: center;
    border: 2px solid var(--color-bg, #121212);
    border-radius: 50%;
    background: #f2c230;
    color: #1b1500;
    font: inherit;
    font-size: 11px;
    font-weight: 800;
    line-height: 1;
    cursor: pointer;
    box-shadow: 0 1px 4px color-mix(in srgb, #000 50%, transparent);
  }

  .gcard__changed:hover {
    transform: scale(1.15);
  }

  .gdraft__row--flash {
    background: color-mix(in srgb, #f2c230 28%, transparent);
    border-radius: 6px;
    outline: 1px solid #f2c230;
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

  .gcard__body :global(> :last-child) {
    margin-bottom: 0;
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

  /* Narrow (portrait) screens: the controls form an even grid and the page
     scrolls, so the map field can be tall instead of squeezed under them. */
  @media (max-width: 720px) {
    .graph {
      overflow-y: auto;
      overflow-x: hidden;
    }

    .graph__tools {
      display: grid;
      grid-template-columns: repeat(6, 1fr);
      gap: 0.5rem 0.6rem;
      align-items: stretch;
      flex: 0 0 auto;
    }

    .graph__pick {
      grid-column: 1 / -1;
    }

    .graph__tools > .graph__ctl,
    .graph__tools > .graph__filter {
      grid-column: span 3;
      min-width: 0;
    }

    .graph__tools > .graph__edge {
      grid-column: span 2;
      justify-content: center;
      gap: 0.25rem;
      white-space: nowrap;
    }

    .graph__edge svg {
      width: 14px;
    }

    .graph__tools > .graph__btn {
      grid-column: span 3;
    }

    .graph__tools > .graph__ctl {
      justify-content: space-between;
    }

    .graph__tools > .graph__ctl select {
      flex: 1 1 auto;
      min-width: 0;
    }

    .graph__tools > .graph__filter summary {
      text-align: center;
      white-space: nowrap;
    }

    .graph__stage {
      flex: 0 0 auto;
      height: max(26rem, 68vh);
    }
  }
</style>
