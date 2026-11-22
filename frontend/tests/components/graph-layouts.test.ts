import { expect, test } from 'bun:test'

import {
  applyLayoutState,
  captureLayout,
  createGraphLayouts,
  type LayoutDeps,
  type LayoutState,
} from '@/pages/graph/model/layouts.svelte.ts'
import { layoutClientId } from '@/shared/api/graph.ts'

type Spot = { x: number; y: number }

/** Карта-заглушка: то, что расклад читает и пишет, без поля и сети. */
function fakeMap(
  init: Partial<{ center: { type: string | null; id: number | null }; size: { w: number; h: number } }> = {}
) {
  const st = {
    center: init.center ?? { type: null as string | null, id: null as number | null },
    roots: [] as { type: string; id: number }[],
    depth: 2,
    layout: 'tree',
    prefs: {
      questStatuses: new Set(['active']),
      shownTypes: new Set(['quest', 'note']),
      hidden: new Set<string>(),
      expandKeys: [] as string[],
      sizes: {} as Record<string, { w: number; h: number }>,
    },
    folds: { expanded: new Set<string>(), loose: new Set<string>() },
    nodes: [] as { key: string; x: number; y: number; draft?: unknown }[],
    saved: new Map<string, Spot>(),
    cam: { x: 0, y: 0, k: 1 },
    size: init.size ?? { w: 1000, h: 600 },
    rootWrites: 0,
    laidOut: 0,
  }
  const deps: LayoutDeps = {
    center: () => st.center,
    setCenter: (c) => {
      st.center = c
      // Смена центра забывает позиции и раскрытия, как на настоящей карте.
      st.saved.clear()
      st.prefs.expandKeys = []
    },
    clearCenter: async () => {
      st.center = { type: null, id: null }
    },
    roots: () => st.roots,
    writeRoots: async (r) => {
      st.roots = r
      st.rootWrites++
    },
    loadDraft: async () => {},
    depth: { get: () => st.depth, set: (d) => (st.depth = d) },
    layout: { get: () => st.layout, set: (l) => (st.layout = l) },
    prefs: {
      get questStatuses() {
        return st.prefs.questStatuses
      },
      get shownTypes() {
        return st.prefs.shownTypes
      },
      get hidden() {
        return st.prefs.hidden
      },
      get expandKeys() {
        return st.prefs.expandKeys
      },
      get sizes() {
        return st.prefs.sizes
      },
      restoreSizes: (s) => {
        st.prefs.sizes = (s ?? {}) as typeof st.prefs.sizes
      },
      restore: (s) => {
        if (s.statuses?.length) st.prefs.questStatuses = new Set(s.statuses)
        if (s.types?.length) st.prefs.shownTypes = new Set(s.types)
        st.prefs.hidden = new Set(s.hidden ?? [])
        st.prefs.expandKeys = s.expand ?? []
      },
    },
    folds: {
      get expanded() {
        return st.folds.expanded
      },
      get loose() {
        return st.folds.loose
      },
      restore: (s) => {
        st.folds.expanded = new Set(s.expanded ?? [])
        st.folds.loose = new Set(s.loose ?? [])
      },
    },
    nodes: () => st.nodes,
    saved: st.saved,
    cam: { get: () => st.cam, set: (c) => (st.cam = c) },
    stageSize: () => st.size,
    settled: () => true,
    applyLayout: () => {
      st.laidOut++
    },
  }
  return { st, deps }
}

test('расклад: снятая карта встаёт на чистую целиком — центр, фильтры, раскрытое, позиции и вид', async () => {
  const a = fakeMap({ center: { type: 'quest', id: 7 } })
  a.st.roots = [{ type: 'note', id: 3 }]
  a.st.depth = 3
  a.st.layout = 'radial'
  a.st.prefs.questStatuses = new Set(['active', 'completed'])
  a.st.prefs.hidden = new Set(['note:9'])
  a.st.prefs.expandKeys = ['quest:7>note']
  a.st.folds.loose = new Set(['quest:7'])
  a.st.nodes = [
    { key: 'quest:7', x: 10.04, y: 20 },
    { key: 'note:3', x: -50, y: 80 },
    { key: 't12', x: 1, y: 1, draft: true },
  ]
  a.st.saved.set('cluster:quest:7', { x: 300, y: 40 })
  a.st.cam = { x: 400, y: 200, k: 2 }
  a.st.prefs.sizes = { 'note:3': { w: 500, h: 400 } }
  const state = JSON.parse(JSON.stringify(captureLayout(a.deps))) as LayoutState

  expect(state.positions['quest:7']).toEqual({ x: 10, y: 20 })
  expect(state.positions['cluster:quest:7']).toEqual({ x: 300, y: 40 })
  expect(state.positions.t12).toBeUndefined()

  // Другое окно: камера ставится по точке мира в центре поля, а не по пикселям.
  const b = fakeMap({ size: { w: 500, h: 300 } })
  await applyLayoutState(b.deps, state)
  expect(b.st.center).toEqual({ type: 'quest', id: 7 })
  expect(b.st.roots).toEqual([{ type: 'note', id: 3 }])
  expect(b.st.depth).toBe(3)
  expect(b.st.layout).toBe('radial')
  expect([...b.st.prefs.questStatuses].sort()).toEqual(['active', 'completed'])
  expect([...b.st.prefs.hidden]).toEqual(['note:9'])
  expect(b.st.prefs.expandKeys).toEqual(['quest:7>note'])
  expect([...b.st.folds.loose]).toEqual(['quest:7'])
  expect(b.st.saved.get('note:3')).toEqual({ x: -50, y: 80 })
  expect(b.st.prefs.sizes).toEqual({ 'note:3': { w: 500, h: 400 } })
  expect(b.st.laidOut).toBe(1)
  // Центр поля a: ((500-400)/2, (300-200)/2) = (50, 50) в мире; в окне b он тоже в центре.
  expect(b.st.cam.k).toBe(2)
  expect((250 - b.st.cam.x) / b.st.cam.k).toBeCloseTo(50)
  expect((150 - b.st.cam.y) / b.st.cam.k).toBeCloseTo(50)
})

test('расклад без центра очищает карту; те же доп. центры не переписываются', async () => {
  const m = fakeMap({ center: { type: 'quest', id: 1 } })
  const empty = captureLayout(fakeMap().deps)
  await applyLayoutState(m.deps, empty)
  expect(m.st.center).toEqual({ type: null, id: null })
  expect(m.st.rootWrites).toBe(0)
})

test('zones and entity manifest survive capture/restore; old layouts clear zones', async () => {
  const a = fakeMap()
  const zone = { id: 'study', title: 'Учёба', color: '#6a8fbf', x: -20, y: 40, w: 600, h: 400 }
  a.deps.prefs.zones = [zone]
  a.deps.entities = () => [{ key: 'note:9', title: 'Заметка' }]
  const state = captureLayout(a.deps)
  expect(state.entities).toEqual([{ key: 'note:9', title: 'Заметка' }])
  expect(state.zones).toEqual([zone])
  const b = fakeMap()
  let restored: unknown = null
  b.deps.prefs.restoreZones = (z) => {
    restored = z
  }
  await applyLayoutState(b.deps, state)
  expect(restored).toEqual([zone])
  delete state.zones
  await applyLayoutState(b.deps, state)
  expect(restored).toEqual([])
})

test('restoring roots never waits for content or restores the camera after user movement', async () => {
  const { st, deps } = fakeMap()
  let release!: () => void
  deps.settled = () => false
  deps.writeRoots = async (roots) => {
    await new Promise<void>((resolve) => {
      release = resolve
    })
    st.roots = roots
  }
  const state = captureLayout(deps)
  state.roots = [{ type: 'note', id: 8 }]
  state.view = { cx: 10, cy: 20, k: 0.2 }
  const pending = applyLayoutState(deps, state)
  st.cam = { x: 77, y: 88, k: 0.12 }
  release()
  await pending
  expect(st.cam).toEqual({ x: 77, y: 88, k: 0.12 })
}, 1000)

function board(id: number): LayoutState {
  return {
    v: 1,
    draft_scope: `scope-${id}`,
    center: { type: 'note', id },
    roots: [],
    depth: 1,
    layout: 'free',
    statuses: ['active'],
    types: ['note'],
    hidden: [],
    expand: [],
    folds: { expanded: [], loose: [] },
    positions: {},
    view: { cx: 0, cy: 0, k: 1 },
  }
}

test('чужой расклад появляется в списке; своё эхо и несохранённая правка не затирают карту', async () => {
  const { st, deps } = fakeMap()
  let remote = board(9)
  const layouts = createGraphLayouts(deps, { readState: async () => remote })
  const tell = (detail: Record<string, unknown>) => window.dispatchEvent(new CustomEvent('quests:layout', { detail }))
  const settled = async () => {
    await new Promise((r) => setTimeout(r, 0))
    for (let i = 0; i < 30 && layouts.busy; i++) await new Promise((r) => setTimeout(r, 0))
    if (layouts.busy) throw new Error('расклад не отпустил очередь')
  }
  tell({ kind: 'graph_layout_created', layout_id: 5, title: 'Сессия 2', client: 'other' })
  expect(layouts.items.find((i) => i.id === 5)?.title).toBe('Сессия 2')
  tell({ kind: 'graph_layout_updated', layout_id: 5, title: 'Дом', client: layoutClientId })
  expect(layouts.items.find((i) => i.id === 5)?.title).toBe('Сессия 2')
  tell({ kind: 'graph_layout_updated', layout_id: 5, title: 'Дом', client: 'other' })
  expect(layouts.items.find((i) => i.id === 5)?.title).toBe('Дом')

  await layouts.open(5)
  expect(st.center).toEqual({ type: 'note', id: 9 })
  remote = board(4)
  tell({ kind: 'graph_layout_updated', layout_id: 5, title: 'Дом', state: true, client: layoutClientId })
  await settled()
  expect(st.center).toEqual({ type: 'note', id: 9 })

  st.nodes = [{ key: 'note:9', x: 12, y: 12 }]
  tell({ kind: 'graph_layout_updated', layout_id: 5, title: 'Дом', state: true, client: 'other' })
  await settled()
  expect(st.center).toEqual({ type: 'note', id: 9 })

  st.nodes = []
  tell({ kind: 'graph_layout_updated', layout_id: 5, title: 'Дом', state: true, client: 'other' })
  await settled()
  expect(st.center).toEqual({ type: 'note', id: 4 })

  tell({ kind: 'graph_layout_deleted', layout_id: 5, client: 'other' })
  await settled()
  expect(layouts.items.some((i) => i.id === 5)).toBe(false)
  expect(typeof layouts.current).toBe('string')
  layouts.dispose()
})
