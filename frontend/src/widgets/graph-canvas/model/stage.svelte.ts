// Поле карты: из загруженного графа и черновика собирается видимая часть, измеряются карточки, кладётся раскладка.
// Камера, выделение и «убрать с карты» живут здесь же — странице остаётся только связать контроллеры.
import { tick, untrack } from 'svelte'

import {
  applyDraft,
  arrangeView,
  CENTERABLE,
  borderMarks,
  childKeysOf,
  drawableEdges,
  fitCamera,
  hiddenNeighbors,
  minimapModel,
  outgoingRefKeys,
  parentNoteOf,
  placedGroups,
  planHide,
  prepareGraph,
  stepOpenTarget,
  visibleGraph,
  type GraphGroupView,
  type GraphNodeView,
} from '@/entities/graph'
import { putGraphDraftRoots } from '@/shared/api/graph.ts'
import type { DraftOp, Graph } from '@/shared/api/store.gen'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Spot = { x: number; y: number }
type Root = { type: string; id: number }
type ViewEdge = { from: string; to: string; kind: string; draft?: string; op?: number }
type LaidOut = { nodes: GraphNodeView[]; edges: ViewEdge[]; groups?: GraphGroupView[] }
type Cam = { x: number; y: number; k: number }

export function createGraphStage(deps: {
  getTrees?: () => { key: string; keys: string[]; layout: string }[]
  getActive: () => boolean
  getData: () => Graph | null
  getDraft: () => { ops: DraftOp[]; roots: Root[] }
  getDepth: () => number
  getQuests: () => { id: number; status?: string }[]
  prefs: {
    hidden: Set<string>
    questStatuses: Set<string>
    shownTypes: Set<string>
    expandKeys: string[]
    sizes: Record<string, { w: number; h: number }>
    saveHidden: (next: Set<string>) => void
    setExpand: (next: string[]) => void
  }
  saved: Map<string, Spot>
  folds: {
    expanded: Set<string>
    loose: Set<string>
    expand: (node: { key: string; x: number; y: number }) => void
    forgetPositions: () => void
  }
  initialLayout: string
  getCam: () => Cam
  setCam: (cam: Cam) => void
  clearMarquee: () => void
  loadDraft: () => Promise<void>
  writeRoots?: (roots: Root[]) => Promise<unknown>
}) {
  const writeRoots = deps.writeRoots ?? putGraphDraftRoots

  let view = $state<LaidOut | null>(null)
  let ready = $state(false)
  let el = $state<HTMLDivElement | null>(null)
  let stageSize = $state({ w: 0, h: 0 })
  let showEdge = $state<Record<string, boolean>>({ structure: true, reference: true, dependency: true })
  let layoutMode = $state(deps.initialLayout)
  let selectMode = $state(false)
  let picked = $state<Set<string>>(new Set())
  let selectedKey = $state<string | null>(null)
  /** An edge picked in its menu: it and both its ends stand out (the rest is dimmed) until Esc, the same menu item again, a pick of another card or a new centre. Panning and zooming keep it. */
  let lit = $state<{ from: string; to: string; kind: string } | null>(null)
  /** Stacking order of touched cards: the last one touched has the highest number. */
  let front = $state<Record<string, number>>({})
  let frontSeq = 0
  let ownerless = $state<Set<string>>(new Set())
  let touched = $state<Map<string, number[]>>(new Map())
  let filteredOut = $state(0)
  let flashOp = $state<number | null>(null)
  let full = $state(false)
  let buildTick = $state(0)
  let buildSeq = 0
  let buildLater = false
  let lastFitKey = ''
  let flashTimer = 0
  let nativeFull = false
  let mapView: LaidOut | null | undefined
  let mapCache = new Map<string, GraphNodeView>()
  let groupView: LaidOut | null | undefined
  let groupCache: GraphGroupView[] = []

  function nodes(): GraphNodeView[] {
    return view?.nodes ?? []
  }
  function edges(): ViewEdge[] {
    return view?.edges ?? []
  }
  /** Пока view тот же объект, повторные чтения отдают тот же Map. */
  function nodesByKey() {
    if (mapView !== view) {
      mapView = view
      mapCache = new Map((view?.nodes ?? []).map((n) => [n.key, n]))
    }
    return mapCache
  }
  function groupsNow() {
    if (groupView !== view) {
      groupView = view
      groupCache = placedGroups(view?.groups ?? [], nodes())
    }
    return groupCache
  }

  function touch() {
    if (view) view = { ...view }
  }

  function applyLayout() {
    if (!view) return
    const trees = deps.getTrees?.() ?? []
    if (!trees.length) {
      view = { ...view, groups: arrangeView(view, layoutMode, { loose: deps.folds.loose, saved: deps.saved }) }
      return
    }
    const rootKeys = new Set(trees.map((t) => t.key))
    const assigned = new Set<string>()
    const groups: GraphGroupView[] = []
    let right = 0
    for (const tree of trees) {
      const keys = new Set(tree.keys)
      const nodes = view.nodes.filter(
        (n) =>
          !assigned.has(n.key) &&
          (!rootKeys.has(n.key) || n.key === tree.key) &&
          (keys.has(n.key) || (n.type === 'cluster' && keys.has(n.key.slice(8))))
      )
      if (!nodes.length) continue
      const localKeys = new Set(nodes.map((n) => n.key))
      const local = { nodes, edges: view.edges.filter((e) => localKeys.has(e.from) && localKeys.has(e.to)) }
      const treeGroups = arrangeView(local, tree.layout, { loose: deps.folds.loose, saved: new Map() })
      const left = Math.min(...nodes.map((n) => n.x - n.w / 2))
      for (const n of nodes) {
        n.x += right - left
        assigned.add(n.key)
      }
      right = Math.max(...nodes.map((n) => n.x + n.w / 2)) + 160
      // Reapply hand positions only after the tree has its place on the canvas.
      for (const n of nodes) {
        const p = deps.saved.get(n.key)
        if (p) {
          n.x = p.x
          n.y = p.y
        }
      }
      groups.push(...treeGroups)
    }
    const rest = view.nodes.filter((n) => !assigned.has(n.key))
    if (rest.length)
      groups.push(
        ...arrangeView({ nodes: rest, edges: [] }, layoutMode, { loose: deps.folds.loose, saved: deps.saved })
      )
    view = { ...view, groups }
  }

  function fit() {
    const node = el
    if (!view || !node) return
    const r = node.getBoundingClientRect()
    const cam = fitCamera(view.nodes, { w: r.width, h: r.height })
    if (cam) deps.setCam(cam)
  }

  /** Спрятать карточки, измерить, разложить, показать. Камера вписывается только при новом центре или глубине. */
  async function rebuild(g: { root?: string; nodes: GraphNodeView[]; edges: ViewEdge[] }, open: Set<string>) {
    const mine = ++buildSeq
    const wasReady = untrack(() => ready)
    ready = false
    // The sizes are put on the plain nodes before they become state: reading `view` back here would make this effect depend on its own write.
    const fresh = prepareGraph(g, open, deps.folds.loose)
    for (const n of fresh.nodes) {
      const s = n.type === 'attachment' || n.type === 'cluster' ? undefined : deps.prefs.sizes[n.key]
      if (s) {
        n.sw = s.w
        n.sh = s.h
      }
    }
    untrack(() => {
      for (const n of wasReady ? (view?.nodes ?? []) : [])
        if (!deps.saved.has(n.key)) deps.saved.set(n.key, { x: n.x, y: n.y })
    })
    view = fresh
    await tick()
    await tick()
    if (mine !== buildSeq || !el || !view) return
    for (const n of view.nodes) {
      const card = el.querySelector(`[data-gkey="${CSS.escape(n.key)}"]`)
      if (card instanceof HTMLElement) {
        n.w = card.offsetWidth
        n.h = card.offsetHeight
      }
    }
    applyLayout()
    ready = true
    await tick()
    const fitKey = `${g.root}|${deps.getTrees ? '' : deps.getDepth()}`
    if (fitKey !== lastFitKey) {
      lastFitKey = fitKey
      fit()
    }
  }

  /**
   * Собрать видимую карту. Пока вкладка скрыта, карточки не измерить: запоминаем, что надо построить,
   * и строим, когда вкладку снова покажут (`poke`).
   */
  function compose() {
    const g = deps.getData()
    void deps.getTrees?.()
    const open = deps.folds.expanded
    void deps.folds.loose
    const draft = deps.getDraft()
    if (!g) return
    const over = applyDraft(g, draft.ops)
    ownerless = over.ownerless
    touched = over.touched
    const vis = visibleGraph({
      root: g.root,
      nodes: over.nodes,
      edges: over.edges,
      draftRoots: draft.roots,
      hidden: deps.prefs.hidden,
      questStatuses: deps.prefs.questStatuses,
      shownTypes: deps.prefs.shownTypes,
    })
    filteredOut = vis.filteredOut
    void buildTick
    if (!untrack(() => deps.getActive())) {
      buildLater = true
      return
    }
    void rebuild({ ...g, nodes: vis.nodes, edges: vis.edges }, open)
  }

  function poke() {
    if (deps.getActive() && buildLater) {
      buildLater = false
      buildTick += 1
    }
  }

  function relayout() {
    deps.folds.forgetPositions()
    applyLayout()
    void tick().then(fit)
  }

  function watchSize(node: HTMLElement) {
    const ro = new ResizeObserver(() => {
      const w = node.clientWidth
      const h = node.clientHeight
      const prev = untrack(() => stageSize)
      if (prev.w && prev.h && w && h) {
        const c = untrack(() => deps.getCam())
        deps.setCam({ ...c, x: c.x + (w - prev.w) / 2, y: c.y + (h - prev.h) / 2 })
      }
      if (w && h) stageSize = { w, h }
    })
    ro.observe(node)
    return () => ro.disconnect()
  }

  function statusOff(key: string) {
    if (!key.startsWith('quest:')) return false
    const quest = deps.getQuests().find((q) => q.id === Number(key.slice('quest:'.length)))
    return Boolean(quest) && !deps.prefs.questStatuses.has(quest?.status ?? '')
  }

  function openMore(n: { key: string; more?: Record<string, string | string[] | undefined> }, type: string | number) {
    const kind = String(type)
    const back = hiddenNeighbors(deps.getData()?.edges ?? [], deps.prefs.hidden).get(n.key)?.[kind]
    if (back?.length) {
      const next = new Set(deps.prefs.hidden)
      for (const k of back) next.delete(k)
      deps.prefs.saveHidden(next)
    }
    const bucket = n.more?.[kind]
    if (Array.isArray(bucket) && bucket.length) {
      const keys = deps.prefs.expandKeys
      const key = `${n.key}>${kind}`
      if (!keys.includes(key) && !keys.includes(n.key)) deps.prefs.setExpand([...keys, key])
    }
  }

  function toggleSelectMode() {
    selectMode = !selectMode
    if (!selectMode) picked = new Set()
    deps.clearMarquee()
  }

  /** Ctrl+A: selection mode on and every card on the field picked. */
  function selectAll() {
    selectMode = true
    picked = new Set(nodes().map((n) => n.key))
    deps.clearMarquee()
  }

  /** Light the edge up; the same edge once more turns it off. */
  function highlight(edge: { from: string; to: string; kind: string }) {
    const same = lit && lit.from === edge.from && lit.to === edge.to && lit.kind === edge.kind
    lit = same ? null : { from: edge.from, to: edge.to, kind: edge.kind }
  }

  function clearLit() {
    lit = null
  }

  /** Selection mode on, what is picked stays. */
  function enterSelect() {
    selectMode = true
  }

  function clearPicked() {
    picked = new Set()
  }

  async function hideKeys(keys: string[]) {
    const plan = planHide({
      keys,
      nodes: nodes(),
      edges: edges(),
      rootKey: deps.getData()?.root,
      hidden: deps.prefs.hidden,
      roots: deps.getDraft().roots,
    })
    // Центр снят и прятать больше нечего: страницу просят очистить карту, сами поля не трогаем.
    if (plan.clearAll) return true
    deps.prefs.saveHidden(plan.hidden)
    if (plan.rootsChanged) {
      try {
        await writeRoots(plan.roots)
        await deps.loadDraft()
      } catch {
        /* toast already shown */
      }
    }
    if (plan.skippedDraft) toast('Карточки черновика не скрываются: уберите их из журнала', { kind: 'info' })
    return false
  }

  async function hidePicked() {
    const keys = [...picked]
    picked = new Set()
    return hideKeys(keys)
  }

  function select(n: { type: string; key: string; x: number; y: number }) {
    if (n.type === 'cluster') {
      deps.folds.expand(n)
      return
    }
    selectedKey = n.key
    lit = null
  }

  function showChanges(key: string) {
    const ops = touched.get(key)
    if (!ops?.length) return false
    flashOp = ops[ops.length - 1] ?? null
    clearTimeout(flashTimer)
    flashTimer = window.setTimeout(() => {
      flashOp = null
    }, 2800)
    void tick().then(() => {
      document.querySelector(`.gdraft__list [data-op="${flashOp}"]`)?.scrollIntoView({ block: 'nearest' })
    })
    return true
  }

  function toggleFull() {
    full = !full
    if (full) {
      const root = document.documentElement
      if (!document.fullscreenElement && root.requestFullscreen) {
        root.requestFullscreen().then(
          () => {
            nativeFull = true
          },
          () => {}
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

  function leaveIfHidden(active: boolean) {
    if (!active && untrack(() => full)) toggleFull()
  }

  /** Середина миникарты становится серединой поля. */
  function miniGo(e: { currentTarget: HTMLElement; clientX: number; clientY: number }) {
    const m = minimapModel({ nodes: nodes(), ready, cam: deps.getCam(), stage: stageSize })
    if (!m) return
    const r = e.currentTarget.getBoundingClientRect()
    const cam = deps.getCam()
    const wx = (e.clientX - r.left) / m.sc + m.x0
    const wy = (e.clientY - r.top) / m.sc + m.y0
    deps.setCam({ ...cam, x: stageSize.w / 2 - wx * cam.k, y: stageSize.h / 2 - wy * cam.k })
  }

  function collapseKey(g: { type: string; owner: string }) {
    return g.type === 'attachment' && deps.folds.expanded.has(`cluster:${g.owner}`) ? `cluster:${g.owner}` : null
  }

  const map = () => nodesByKey()

  return {
    get el() {
      return el
    },
    set el(v: HTMLDivElement | null) {
      el = v
    },
    get view() {
      return view
    },
    get ready() {
      return ready
    },
    get nodes() {
      return nodes()
    },
    get edges() {
      return edges()
    },
    get nodeMap() {
      return map()
    },
    get groups() {
      return groupsNow()
    },
    get drawEdges() {
      return drawableEdges({ edges: edges(), nodes: nodes(), groups: groupsNow(), showEdge })
    },
    get front() {
      return front
    },
    /** Brings a card to the foreground (it keeps the order until another one is touched). */
    raise(key: string) {
      if (front[key] === frontSeq && frontSeq > 0) return
      front = { ...front, [key]: ++frontSeq }
    },
    get selectedKey() {
      return selectedKey
    },
    set selectedKey(v: string | null) {
      selectedKey = v
    },
    get selected() {
      return selectedKey ? (map().get(selectedKey) ?? null) : null
    },
    get showEdge() {
      return showEdge
    },
    set showEdge(v: Record<string, boolean>) {
      showEdge = v
    },
    get layoutMode() {
      return layoutMode
    },
    set layoutMode(v: string) {
      layoutMode = v
    },
    get selectMode() {
      return selectMode
    },
    get picked() {
      return picked
    },
    setPicked(s: Set<string>) {
      picked = s
    },
    get ownerless() {
      return ownerless
    },
    get touched() {
      return touched
    },
    get filteredOut() {
      return filteredOut
    },
    get flashOp() {
      return flashOp
    },
    get full() {
      return full
    },
    get stageSize() {
      return stageSize
    },
    get moreMap() {
      const adj = hiddenNeighbors(deps.getData()?.edges ?? [], deps.prefs.hidden)
      const shown = deps.prefs.shownTypes
      const marks = new Map<string, { type: string; count: number }[]>()
      for (const n of deps.getData()?.nodes ?? []) {
        const row = borderMarks(n, adj, shown, statusOff)
        if (row.length) marks.set(n.key, row)
      }
      return marks
    },
    get mini() {
      return minimapModel({ nodes: nodes(), ready, cam: deps.getCam(), stage: stageSize })
    },
    get canRecenter() {
      const n = selectedKey ? map().get(selectedKey) : null
      return Boolean(n && n.key !== deps.getData()?.root && CENTERABLE.has(n.type) && !n.draft)
    },
    touch,
    /** Разложить заново с тем, что сейчас в `saved` (расклад поставил позиции карточек). */
    applyLayout,
    compose,
    poke,
    fit,
    relayout,
    watchSize,
    openMore,
    toggleSelectMode,
    clearPicked,
    enterSelect,
    highlight,
    clearLit,
    get lit() {
      return lit
    },
    selectAll,
    hidePicked,
    hideKeys,
    select,
    showChanges,
    toggleFull,
    onFullscreenChange,
    leaveIfHidden,
    miniGo,
    collapseKey,
    openTarget: (n: { draft?: unknown; type: string; id?: number | string; key: string } | null) =>
      stepOpenTarget(n, deps.getData()),
    childKeys: (n: { key: string }, type: string) => childKeysOf(edges(), map() as Map<string, unknown>, n.key, type),
    parentNote: (n: { key: string }) => parentNoteOf(edges(), n.key),
    outgoingRefs: (n: { key: string }) => outgoingRefKeys(edges(), map() as Map<string, unknown>, n.key),
    titleOf: (key: string) => map().get(key)?.title ?? key,
  }
}
