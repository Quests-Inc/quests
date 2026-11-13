// Сохранённые расклады карты (личные, на сервере): что карта показывает и где стоят её карточки.
// Сервер хранит состояние как есть; здесь его собирают со страницы и ставят обратно.
import { tick } from 'svelte'

import { ENTITY_TYPES, type LayoutEntity, type GraphZone } from '@/entities/graph'
import {
  setGraphDraftScope,
  createGraphLayout,
  deleteGraphLayout,
  getGraphLayout,
  listGraphLayouts,
  updateGraphLayout,
} from '@/shared/api/graph.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Spot = { x: number; y: number }
type Cam = { x: number; y: number; k: number }
type Root = { type: string; id: number }

/** Что лежит в раскладе. Камера — точка мира в центре поля и масштаб, чтобы не зависеть от размера окна. */
export type LayoutState = {
  draft_scope?: string
  entities?: LayoutEntity[]
  zones?: GraphZone[]
  trees?: Record<string, { depth: number; layout: string }>
  v: 1
  center: { type: string | null; id: number | null }
  roots: Root[]
  depth: number
  layout: string
  statuses: string[]
  types: string[]
  hidden: string[]
  expand: string[]
  folds: { expanded: string[]; loose: string[] }
  positions: Record<string, Spot>
  /** Sizes of the cards the person stretched. */
  sizes?: Record<string, { w: number; h: number }>
  view: { cx: number; cy: number; k: number } | null
}

export type LayoutDeps = {
  center: () => { type: string | null; id: number | null }
  setCenter: (c: { type: string; id: number }) => void
  clearCenter: () => Promise<void>
  roots: () => Root[]
  writeRoots: (roots: Root[]) => Promise<unknown>
  loadDraft: () => Promise<void>
  depth: { get: () => number; set: (d: number) => void }
  layout: { get: () => string; set: (l: string) => void }
  prefs: {
    zones?: GraphZone[]
    restoreZones?: (s: unknown) => void
    trees?: Record<string, { depth: number; layout: string }>
    restoreTrees?: (s: unknown) => void
    questStatuses: Set<string>
    shownTypes: Set<string>
    hidden: Set<string>
    expandKeys: string[]
    sizes: Record<string, { w: number; h: number }>
    restoreSizes: (s: unknown) => void
    restore: (s: { statuses?: string[]; types?: string[]; hidden?: string[]; expand?: string[] }) => void
  }
  folds: { expanded: Set<string>; loose: Set<string>; restore: (s: { expanded?: string[]; loose?: string[] }) => void }
  titleOf?: (key: string) => string
  entities?: () => LayoutEntity[]
  nodes: () => { key: string; title?: string; x: number; y: number; draft?: unknown }[]
  saved: Map<string, Spot>
  cam: { get: () => Cam; set: (c: Cam) => void }
  stageSize: () => { w: number; h: number }
  /** Карта догрузилась и разложилась (после смены центра, глубины, раскрытий). */
  settled: () => boolean
  applyLayout: () => void
  addRoot?: (type: string, id: number) => Promise<void>
}

const depsEntities = (d: LayoutDeps) =>
  d.entities?.() ??
  d
    .nodes()
    .filter((n) => !n.draft && !n.key.startsWith('cluster:'))
    .map((n) => ({ key: n.key, title: n.title || n.key }))

const round = (n: number) => Math.round(n * 10) / 10

export function captureLayout(d: LayoutDeps): LayoutState {
  const positions: Record<string, Spot> = {}
  for (const [k, p] of d.saved) positions[k] = { x: round(p.x), y: round(p.y) }
  for (const n of d.nodes()) if (!n.draft) positions[n.key] = { x: round(n.x), y: round(n.y) }
  const cam = d.cam.get()
  const size = d.stageSize()
  const view =
    size.w && size.h && cam.k
      ? { cx: round((size.w / 2 - cam.x) / cam.k), cy: round((size.h / 2 - cam.y) / cam.k), k: cam.k }
      : null
  return {
    entities: depsEntities(d),
    zones: d.prefs.zones?.map((z) => ({ ...z })) ?? [],
    trees: d.prefs.trees ? { ...d.prefs.trees } : undefined,
    v: 1,
    center: { ...d.center() },
    roots: d.roots().map((r) => ({ type: r.type, id: r.id })),
    depth: d.depth.get(),
    layout: d.layout.get(),
    statuses: [...d.prefs.questStatuses],
    types: [...d.prefs.shownTypes],
    hidden: [...d.prefs.hidden],
    expand: [...d.prefs.expandKeys],
    folds: { expanded: [...d.folds.expanded], loose: [...d.folds.loose] },
    positions,
    sizes: { ...d.prefs.sizes },
    view,
  }
}

const sameRoots = (a: Root[], b: Root[]) =>
  a.length === b.length && a.every((r, i) => r.type === b[i].type && r.id === b[i].id)

async function until(ok: () => boolean, ms = 6000) {
  const end = Date.now() + ms
  while (!ok() && Date.now() < end) await new Promise((r) => setTimeout(r, 50))
}

/** Поставить карту в сохранённое состояние. Смена центра забывает позиции и раскрытия — поэтому они ставятся после неё. */
export async function applyLayoutState(d: LayoutDeps, s: LayoutState) {
  if (!sameRoots(d.roots(), s.roots ?? [])) {
    await d.writeRoots(s.roots ?? [])
    await d.loadDraft()
  }
  const c = d.center()
  if (!s.center?.type || !s.center.id) {
    if (c.type) await d.clearCenter()
  } else if (c.type !== s.center.type || c.id !== s.center.id) {
    d.setCenter({ type: s.center.type, id: s.center.id })
  }
  await tick()
  await tick()
  d.depth.set(s.depth || 2)
  d.layout.set(s.layout || 'tree')
  d.prefs.restoreTrees?.(
    s.trees ??
      Object.fromEntries(
        [s.center, ...(s.roots ?? [])]
          .filter((r) => r?.type && r.id)
          .map((r) => [`${r.type}:${r.id}`, { depth: s.depth || 2, layout: s.layout || 'tree' }])
      )
  )
  d.prefs.restore({
    statuses: s.statuses ?? ['active'],
    types: s.types ?? ENTITY_TYPES,
    hidden: s.hidden ?? [],
    expand: s.expand ?? [],
  })
  d.prefs.restoreSizes(s.sizes ?? {})
  d.prefs.restoreZones?.(s.zones ?? [])
  d.folds.restore(s.folds ?? {})
  d.saved.clear()
  for (const [k, p] of Object.entries(s.positions ?? {})) d.saved.set(k, p)
  await tick()
  if (s.center?.id || s.roots?.length || d.nodes().length) await until(d.settled)
  d.applyLayout()
  const size = d.stageSize()
  if (s.view && size.w && size.h)
    d.cam.set({ k: s.view.k, x: size.w / 2 - s.view.cx * s.view.k, y: size.h / 2 - s.view.cy * s.view.k })
}

const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** Every visible board owns a draft UUID. Temporary boards exist only in this controller's memory. */
export function createGraphLayouts(deps: LayoutDeps, options: { rememberActive?: boolean } = {}) {
  type Tab = { id: number | string; title: string; updated_at: string; temporary?: boolean }
  let items = $state<Tab[]>([])
  let loaded = $state(false)
  let current = $state<number | string>(crypto.randomUUID())
  let scope = String(current)
  setGraphDraftScope(scope)
  let busy = $state(false)
  let saving = $state(false)
  let failure = $state('')
  const snapshots = new Map<string, LayoutState>()
  const remember = (id: number | string) => {
    if (!options.rememberActive) return
    try {
      const url = new URL(location.href)
      if (typeof id === 'number' && url.searchParams.get('tab') === 'graph') {
        if (url.searchParams.get('layout') === String(id)) return
        url.searchParams.set('layout', String(id))
      } else {
        if (!url.searchParams.has('layout')) return
        url.searchParams.delete('layout')
      }
      history.replaceState(history.state, '', url)
    } catch {}
  }
  let restoreId = 0
  if (options.rememberActive)
    try {
      restoreId = Number(new URL(location.href).searchParams.get('layout'))
    } catch {}
  let untouched = true
  let lastSaved = ''
  let timer: ReturnType<typeof setTimeout> | undefined
  let pending: Promise<void> = Promise.resolve()
  let writes: Promise<void> = Promise.resolve()

  function nextName(prefix = 'Новая раскладка') {
    let n = 1
    while (items.some((i) => i.title === `${prefix} (${n})`)) n++
    return `${prefix} (${n})`
  }
  items = [
    { id: current, title: nextName('Временная раскладка'), temporary: true, updated_at: new Date().toISOString() },
  ]
  function capture() {
    return { ...captureLayout(deps), draft_scope: scope }
  }
  function empty(): LayoutState {
    return {
      ...capture(),
      entities: [],
      center: { type: null, id: null },
      roots: [],
      zones: [],
      trees: {},
      hidden: [],
      expand: [],
      folds: { expanded: [], loose: [] },
      positions: {},
      sizes: {},
      depth: 2,
      layout: 'tree',
      statuses: ['active'],
      types: [...ENTITY_TYPES],
      view: { cx: 0, cy: 0, k: 1 },
    }
  }
  async function load() {
    const list = await listGraphLayouts()
    items = [...items.filter((i) => i.temporary), ...list]
    loaded = true
  }
  async function persist(force = false) {
    clearTimeout(timer)
    if (typeof current !== 'number' || (!force && !deps.settled())) return
    const id = current,
      state = capture(),
      json = JSON.stringify(state)
    if (json === lastSaved) return
    const task = writes.then(async () => {
      saving = true
      try {
        const saved = await updateGraphLayout(id, { state })
        if (current === id) lastSaved = json
        items = items.map((i) => (i.id === id ? { ...i, updated_at: saved.updated_at } : i))
        failure = ''
      } catch (e) {
        failure = messageOf(e)
        throw e
      } finally {
        saving = false
      }
    })
    writes = task.catch(() => {})
    await task
  }
  function run(fn: () => Promise<void>) {
    const job = pending.then(async () => {
      busy = true
      clearTimeout(timer)
      try {
        await fn()
        failure = ''
      } catch (e) {
        failure = messageOf(e)
        toast(failure, { kind: 'error' })
      } finally {
        busy = false
      }
    })
    pending = job
    return job
  }
  async function stash() {
    untouched = false
    clearTimeout(timer)
    snapshots.set(String(current), capture())
    await persist(true)
    await writes
  }
  async function activate(id: number | string, state: LayoutState) {
    scope = state.draft_scope || crypto.randomUUID()
    setGraphDraftScope(scope)
    current = id
    remember(id)
    await deps.loadDraft()
    await applyLayoutState(deps, state)
    remember(id)
    lastSaved = typeof id === 'number' && state.draft_scope ? JSON.stringify(capture()) : ''
  }
  return {
    get items() {
      return items
    },
    get loaded() {
      return loaded
    },
    get current() {
      return current
    },
    get busy() {
      return busy
    },
    get saving() {
      return saving
    },
    get error() {
      return failure
    },
    get canSave() {
      return !busy && deps.settled()
    },
    async load() {
      try {
        await load()
        if (restoreId && items.some((i) => i.id === restoreId)) {
          const id = restoreId
          restoreId = 0
          await run(async () => {
            const l = await getGraphLayout(id)
            await activate(id, l.state as LayoutState)
          })
        }
      } catch (e) {
        loaded = true
        failure = messageOf(e)
      }
    },
    /** Reactive observer: only saved boards autosave. Temporary state never goes into browser storage. */
    track() {
      if (busy || !deps.settled() || typeof current !== 'number') return
      remember(current)
      const json = JSON.stringify(capture()),
        id = current
      clearTimeout(timer)
      if (json !== lastSaved)
        timer = setTimeout(() => {
          if (current === id && !busy) void persist().catch(() => {})
        }, 700)
    },
    dispose() {
      clearTimeout(timer)
    },
    placePosition(id: number | string, key: string, position: Spot) {
      return run(async () => {
        if (id === current) {
          deps.saved.set(key, position)
          return
        }
        const old =
          snapshots.get(String(id)) ??
          (typeof id === 'number' ? ((await getGraphLayout(id)).state as LayoutState) : undefined)
        if (!old) return
        const state = { ...old, positions: { ...old.positions, [key]: position } }
        if (typeof id === 'number') await updateGraphLayout(id, { state })
        snapshots.set(String(id), state)
      })
    },
    /** Uploads may finish after a switch: place them in their originating board, never the current one by accident. */
    placeEntity(id: number | string, ref: { type: string; id: number; title: string }, position: Spot) {
      return run(async () => {
        const key = `${ref.type}:${ref.id}`
        if (id === current) {
          if (!deps.addRoot) throw new Error('Добавление на карту недоступно')
          await deps.addRoot(ref.type, ref.id)
          deps.saved.set(key, position)
          await tick()
          deps.applyLayout()
          return
        }
        const old =
          snapshots.get(String(id)) ??
          (typeof id === 'number' ? ((await getGraphLayout(id)).state as LayoutState) : undefined)
        if (!old) throw new Error('Файл загружен во вложения, но исходный расклад уже закрыт')
        const hasCenter = !!old.center?.id
        const state: LayoutState = {
          ...old,
          center: hasCenter ? old.center : { type: ref.type, id: ref.id },
          roots: hasCenter ? [...(old.roots ?? []), { type: ref.type, id: ref.id }] : (old.roots ?? []),
          positions: { ...old.positions, [key]: position },
          entities: [...(old.entities ?? []), { key, title: ref.title }],
        }
        if (typeof id === 'number') await updateGraphLayout(id, { state })
        snapshots.set(String(id), state)
      })
    },
    newEmpty(title: string) {
      return run(async () => {
        await stash()
        const state = empty()
        state.draft_scope = crypto.randomUUID()
        const l = await createGraphLayout(title.trim() || nextName(), state)
        await load()
        await activate(l.id, state)
      })
    },
    newTemporary() {
      return run(async () => {
        if (untouched && typeof current === 'string' && !deps.nodes().length && !deps.roots().length) {
          untouched = false
          const state = empty()
          state.draft_scope = scope
          await activate(current, state)
          return
        }
        await stash()
        const id = crypto.randomUUID(),
          state = empty()
        state.draft_scope = id
        items = [
          ...items,
          { id, title: nextName('Временная раскладка'), temporary: true, updated_at: new Date().toISOString() },
        ]
        await activate(id, state)
      })
    },
    saveNew(title: string) {
      return run(async () => {
        const l = await createGraphLayout(
          title.trim() || items.find((i) => i.id === current)?.title || nextName(),
          capture()
        )
        const old = current
        current = l.id
        remember(l.id)
        items = items.filter((i) => i.id !== old)
        await load()
        lastSaved = JSON.stringify(capture())
      })
    },
    overwrite(id: number | string) {
      return run(async () => {
        if (id === current) await persist()
      })
    },
    open(id: number | string) {
      return run(async () => {
        if (id === current) return
        await stash()
        const state = typeof id === 'number' ? ((await getGraphLayout(id)).state as LayoutState) : snapshots.get(id)
        if (state) await activate(id, state)
      })
    },
    rename(id: number | string, title: string) {
      return run(async () => {
        title = title.trim()
        if (!title) return
        if (typeof id === 'number') await updateGraphLayout(id, { title })
        items = items.map((i) => (i.id === id ? { ...i, title } : i))
      })
    },
    remove(id: number | string) {
      return run(async () => {
        if (typeof id === 'number') await deleteGraphLayout(id)
        items = items.filter((i) => i.id !== id)
        snapshots.delete(String(id))
        if (current === id) {
          const next = crypto.randomUUID(),
            state = empty()
          state.draft_scope = next
          items = [
            ...items,
            { id: next, title: nextName('Временная раскладка'), temporary: true, updated_at: new Date().toISOString() },
          ]
          await activate(next, state)
        }
      })
    },
  }
}
