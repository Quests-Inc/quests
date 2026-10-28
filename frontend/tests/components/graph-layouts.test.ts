import { expect, test } from 'bun:test'

import {
  applyLayoutState,
  captureLayout,
  type LayoutDeps,
  type LayoutState,
} from '@/pages/graph/model/layouts.svelte.ts'

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
