// Сохранённые расклады карты (личные, на сервере): что карта показывает и где стоят её карточки.
// Сервер хранит состояние как есть; здесь его собирают со страницы и ставят обратно.
import { tick } from 'svelte'

import { diffLayoutEntities, type LayoutEntity, type GraphZone } from '@/entities/graph'
import {
  createGraphLayout,
  deleteGraphLayout,
  getGraphLayout,
  listGraphLayouts,
  updateGraphLayout,
  type GraphLayout,
} from '@/shared/api/graph.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Spot = { x: number; y: number }
type Cam = { x: number; y: number; k: number }
type Root = { type: string; id: number }

/** Что лежит в раскладе. Камера — точка мира в центре поля и масштаб, чтобы не зависеть от размера окна. */
export type LayoutState = {
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
  if (s.depth) d.depth.set(s.depth)
  if (s.layout) d.layout.set(s.layout)
  d.prefs.restoreTrees?.(
    s.trees ??
      Object.fromEntries(
        [s.center, ...(s.roots ?? [])]
          .filter((r) => r?.type && r.id)
          .map((r) => [`${r.type}:${r.id}`, { depth: s.depth || 2, layout: s.layout || 'tree' }])
      )
  )
  d.prefs.restore({ statuses: s.statuses, types: s.types, hidden: s.hidden, expand: s.expand })
  d.prefs.restoreSizes(s.sizes ?? {})
  d.prefs.restoreZones?.(s.zones ?? [])
  d.folds.restore(s.folds ?? {})
  d.saved.clear()
  for (const [k, p] of Object.entries(s.positions ?? {})) d.saved.set(k, p)
  await tick()
  await until(d.settled)
  d.applyLayout()
  const size = d.stageSize()
  if (s.view && size.w && size.h)
    d.cam.set({ k: s.view.k, x: size.w / 2 - s.view.cx * s.view.k, y: size.h / 2 - s.view.cy * s.view.k })
}

const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** Список раскладов в боковой панели и действия над ними. */
export function createGraphLayouts(deps: LayoutDeps) {
  let items = $state<GraphLayout[]>([])
  let loaded = $state(false)
  let current = $state<number | null>(null)
  let busy = $state(false)
  let overwriteAsk = $state<{
    id: number
    title: string
    state: LayoutState
    diff: ReturnType<typeof diffLayoutEntities>
    legacy: boolean
  } | null>(null)

  async function load() {
    try {
      items = await listGraphLayouts()
    } catch (e) {
      toast(`Расклады не загрузились: ${messageOf(e)}`, { kind: 'error' })
    } finally {
      loaded = true
    }
  }

  async function run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    busy = true
    try {
      return await fn()
    } catch (e) {
      toast(messageOf(e), { kind: 'error' })
      return undefined
    } finally {
      busy = false
    }
  }

  return {
    get canSave() {
      return !busy && deps.settled()
    },
    get overwriteAsk() {
      return overwriteAsk
    },
    cancelOverwrite() {
      if (!busy) overwriteAsk = null
    },
    async confirmOverwrite() {
      const pending = overwriteAsk
      if (!pending || busy) return
      const l = await run(() => updateGraphLayout(pending.id, { state: pending.state }))
      if (!l) return
      overwriteAsk = null
      current = l.id
      await load()
      toast(`Расклад «${l.title}» обновлён`, { kind: 'success', ttl: 1600 })
    },
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
    load,
    /** Сохранить текущую карту новым раскладом. */
    async saveNew(title: string) {
      if (busy || !deps.settled()) return
      const l = await run(() => createGraphLayout(title, captureLayout(deps)))
      if (!l) return
      current = l.id
      await load()
      toast(`Расклад «${l.title}» сохранён`, { kind: 'success', ttl: 1600 })
    },
    /** Записать текущую карту в существующий расклад. */
    async overwrite(id: number) {
      if (busy || !deps.settled()) return
      const state = captureLayout(deps)
      const l = await run(() => getGraphLayout(id))
      if (!l) return
      const before = l.state as LayoutState
      const diff = diffLayoutEntities(before, state)
      for (const entity of diff.removed)
        if (entity.title === entity.key) entity.title = deps.titleOf?.(entity.key) || entity.key
      overwriteAsk = { id, title: l.title, state, diff, legacy: !before?.entities }
    },
    async open(id: number) {
      await run(async () => {
        const l = await getGraphLayout(id)
        current = id
        await applyLayoutState(deps, l.state as LayoutState)
      })
    },
    async rename(id: number, title: string) {
      if (await run(() => updateGraphLayout(id, { title }))) await load()
    },
    async remove(id: number) {
      if ((await run(() => deleteGraphLayout(id))) === undefined) return
      if (current === id) current = null
      await load()
    },
  }
}
