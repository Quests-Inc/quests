// Центр карты и загрузка графа вокруг него: поиск корня, дополнительные центры, очистка.
import { CENTERABLE } from '@/entities/graph'
import { findEntities } from '@/entities/quest'
import { getGraph, putGraphDraftRoots } from '@/shared/api/graph.ts'
import type { Graph } from '@/shared/api/store.gen'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Center = { type: string | null; id: number | null }
type Root = { type: string; id: number }
type Listed = { id: number; title?: string; status?: string }

const emptyGraph = (depth: number): Graph => ({ root: '', roots: [], nodes: [], edges: [], truncated: false, depth })

function messageOf(e: unknown) {
  if (e instanceof Error && e.message) return e.message
  if (e && typeof e === 'object' && 'message' in e && typeof e.message === 'string' && e.message) return e.message
  return String(e)
}

export function createGraphQuery(deps: {
  getTreeSettings?: (key: string) => { depth: number; layout: string } | undefined
  initTree?: (key: string, depth: number) => void
  getActive: () => boolean
  getRootType: () => string | null
  getRootId: () => number | null
  getNonce: () => number
  getExpandKeys: () => string[]
  getReloadTick: () => number
  getDraftRoots: () => Root[]
  getHiddenSize: () => number
  getQuests: () => Listed[]
  getNotes: () => Listed[]
  getQuestlines: () => Listed[]
  onCenter: (type: string | null, id: number | null) => void
  /** Сменился центр снаружи (другая карточка, nonce): забыть раскрытия и позиции. */
  onRetarget: () => void
  /** Новый центр выбран здесь: забыть позиции и точечные раскрытия, группы вложений не трогать. */
  prepareFresh: () => void
  forgetPositions: () => void
  resetFolds: () => void
  setExpand: (keys: string[]) => void
  saveHidden: (hidden: Set<string>) => void
  cancelLink: () => void
  clearEditing: () => void
  clearSelection: () => void
  loadDraft: () => Promise<void>
  fetchGraph?: typeof getGraph
  writeRoots?: (roots: Root[]) => Promise<unknown>
}) {
  const fetchGraph = deps.fetchGraph ?? getGraph
  const writeRoots = deps.writeRoots ?? putGraphDraftRoots

  let depth = $state(2)
  let center = $state<Center>({ type: deps.getRootType(), id: deps.getRootId() })
  let data = $state<Graph | null>(null)
  let memberships = $state<Record<string, string[]>>({})
  let loading = $state(false)
  let error = $state('')
  let rootQuery = $state('')
  let pickOpen = $state(false)
  let seq = 0
  let lastRootKey = ''
  let lastDataJson = ''

  /** Подтянуть центр к тому, что открыто в журнале. Повтор того же ключа ничего не делает. */
  function syncRoot() {
    const rootType = deps.getRootType()
    const rootId = deps.getRootId()
    if (!deps.getActive() || !rootType || !rootId) return
    const key = `${rootType}:${rootId}:${deps.getNonce()}`
    if (key === lastRootKey) return
    lastRootKey = key
    deps.initTree?.(`${rootType}:${rootId}`, depth)
    center = { type: rootType, id: rootId }
    deps.onRetarget()
  }

  /** Запросить граф. Пока вкладка скрыта — нет. Пустой центр — пустая карта, чтобы черновик всё равно было куда класть. */
  function load() {
    if (!deps.getActive()) return
    if (!center.type || !center.id) {
      seq += 1
      loading = false
      memberships = {}
      lastDataJson = ''
      data = emptyGraph(depth)
      return
    }
    const c = center
    const d = deps.getTreeSettings ? 2 : depth
    const extra = deps.getDraftRoots().filter((r) => !(r.type === c.type && r.id === c.id))
    const open = deps.getExpandKeys().map((k) => {
      const [ref, only] = k.split('>')
      const parts = ref.split(':')
      return { type: parts[0] ?? '', id: Number(parts[1]), only }
    })
    void deps.getNonce()
    void deps.getReloadTick()
    const mine = ++seq
    loading = true
    error = ''
    const roots = [{ type: c.type, id: c.id }, ...extra]
    for (const r of roots) deps.initTree?.(`${r.type}:${r.id}`, d)
    const requests = deps.getTreeSettings
      ? roots.map((r) => fetchGraph(r.type!, r.id!, deps.getTreeSettings!(`${r.type}:${r.id}`)?.depth ?? d, [], open))
      : [fetchGraph(c.type, c.id, d, extra, open)]
    Promise.all(requests)
      .then((graphs) => {
        const first = graphs[0]
        const g = {
          ...first,
          roots: roots.map((r) => `${r.type}:${r.id}`),
          nodes: [...new Map(graphs.flatMap((g) => g.nodes).map((n) => [n.key, n])).values()].map((n) =>
            roots.some((r) => `${r.type}:${r.id}` === n.key) ? { ...n, depth: 0 } : n
          ),
          edges: [...new Map(graphs.flatMap((g) => g.edges).map((e) => [`${e.from}|${e.to}|${e.kind}`, e])).values()],
          truncated: graphs.some((g) => g.truncated),
        }

        if (mine !== seq) return
        memberships = Object.fromEntries(
          graphs.map((g, i) => [`${roots[i].type}:${roots[i].id}`, g.nodes.map((n) => n.key)])
        )
        const json = JSON.stringify(g)
        if (json !== lastDataJson) {
          lastDataJson = json
          data = g
        }
      })
      .catch((e: unknown) => {
        if (mine === seq) error = messageOf(e)
      })
      .finally(() => {
        if (mine === seq) loading = false
      })
  }

  function focus(type: string, id: number) {
    deps.initTree?.(`${type}:${id}`, depth)
    deps.prepareFresh()
    center = { type, id }
    deps.onCenter(type, id)
  }

  function chooseRoot(m: { type: string; id: number }) {
    rootQuery = ''
    pickOpen = false
    focus(m.type, m.id)
  }

  function recenter(n: { type: string; draft?: unknown; id: number | string }) {
    if (!CENTERABLE.has(n.type) || n.draft || typeof n.id !== 'number') return
    focus(n.type, n.id)
  }

  function setCenter(c: { type: string; id: number }) {
    center = { type: c.type, id: c.id }
    deps.onCenter(c.type, c.id)
  }

  async function clearMap() {
    center = { type: null, id: null }
    deps.clearSelection()
    deps.resetFolds()
    deps.cancelLink()
    deps.clearEditing()
    deps.forgetPositions()
    deps.setExpand([])
    deps.saveHidden(new Set())
    try {
      await writeRoots([])
      await deps.loadDraft()
    } catch {
      /* toast already shown */
    }
    deps.onCenter(null, null)
  }

  async function addToMap(type: string, id: number) {
    deps.initTree?.(`${type}:${id}`, depth)
    if (!center.id) {
      deps.forgetPositions()
      center = { type, id }
      deps.onCenter(type, id)
      return
    }
    if (center.type === type && center.id === id) {
      toast('Это уже центр карты', { kind: 'info' })
      return
    }
    const roots = deps.getDraftRoots()
    if (roots.some((r) => r.type === type && r.id === id)) {
      toast('Уже на карте', { kind: 'info' })
      return
    }
    await writeRoots([...roots, { type, id }])
    await deps.loadDraft()
  }

  return {
    get memberships() {
      return memberships
    },
    get depth() {
      return depth
    },
    set depth(v: number) {
      depth = v
    },
    get center() {
      return center
    },
    get data() {
      return data
    },
    get loading() {
      return loading
    },
    get error() {
      return error
    },
    get rootQuery() {
      return rootQuery
    },
    set rootQuery(v: string) {
      rootQuery = v
    },
    get pickOpen() {
      return pickOpen
    },
    set pickOpen(v: boolean) {
      pickOpen = v
    },
    get found() {
      return findEntities(rootQuery, {
        questlines: deps.getQuestlines(),
        quests: deps.getQuests(),
        notes: deps.getNotes(),
        limit: 12,
      })
    },
    get canClear() {
      return Boolean(center.id) || deps.getDraftRoots().length > 0 || deps.getHiddenSize() > 0
    },
    syncRoot,
    load,
    chooseRoot,
    recenter,
    setCenter,
    clearMap,
    addToMap,
  }
}
