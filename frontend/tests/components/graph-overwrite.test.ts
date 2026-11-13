import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import type { LayoutDeps, LayoutState } from '@/pages/graph/model/layouts.svelte.ts'
import GraphLayouts from '@/pages/graph/ui/GraphLayouts.svelte'
let scope = ''
const db = new Map<number, { id: number; title: string; state: LayoutState; updated_at: string }>()
const writes: { id: number; state?: LayoutState }[] = []
mock.module('@/shared/api/graph.ts', () => ({
  setGraphDraftScope: (key: string) => (scope = key),
  listGraphLayouts: async () => [...db.values()],
  getGraphLayout: async (id: number) => db.get(id),
  updateGraphLayout: async (id: number, patch: { state?: LayoutState; title?: string }) => {
    writes.push({ id, ...patch })
    const value = { ...db.get(id)!, ...patch }
    db.set(id, value)
    return value
  },
  createGraphLayout: async (title: string, state: LayoutState) => {
    const l = { id: db.size + 1, title, state, updated_at: new Date().toISOString() }
    db.set(l.id, l)
    return l
  },
  deleteGraphLayout: async (id: number) => {
    db.delete(id)
    return {}
  },
}))
const { createGraphLayouts } = await import('@/pages/graph/model/layouts.svelte.ts')
function map() {
  let center: { type: string | null; id: number | null } = { type: 'note', id: 8 }
  let nodes = [{ key: 'note:8', title: 'Eight', x: 10, y: 20 }]
  const roots = new Map<string, { type: string; id: number }[]>()
  const deps: LayoutDeps = {
    center: () => center,
    setCenter: (c) => {
      center = c
      nodes = [{ key: `${c.type}:${c.id}`, title: 'Eight', x: 10, y: 20 }]
    },
    clearCenter: async () => {
      center = { type: null, id: null }
      nodes = []
    },
    roots: () => roots.get(scope) ?? [],
    writeRoots: async (r) => {
      roots.set(scope, r)
    },
    loadDraft: async () => {},
    depth: { get: () => 2, set: () => {} },
    layout: { get: () => 'tree', set: () => {} },
    prefs: {
      questStatuses: new Set(['active']),
      shownTypes: new Set(['note']),
      hidden: new Set(),
      expandKeys: [],
      sizes: {},
      restoreSizes: () => {},
      restore: () => {},
    },
    folds: { expanded: new Set(), loose: new Set(), restore: () => {} },
    nodes: () => nodes,
    saved: new Map(),
    cam: { get: () => ({ x: 0, y: 0, k: 1 }), set: () => {} },
    stageSize: () => ({ w: 800, h: 600 }),
    settled: () => true,
    applyLayout: () => {},
  }
  return {
    deps,
    get nodes() {
      return nodes
    },
  }
}
test('new boards are empty, temporary state stays separate, saves target only the active board', async () => {
  const m = map(),
    layouts = createGraphLayouts(m.deps),
    temp = layouts.current,
    oldScope = scope
  await layouts.newEmpty('')
  expect(db.get(1)?.title).toBe('Новая раскладка (1)')
  expect(db.get(1)?.state.center).toEqual({ type: null, id: null })
  expect(db.get(1)?.state.entities).toEqual([])
  expect(scope).not.toBe(oldScope)
  await layouts.newEmpty('')
  expect(db.get(2)?.title).toBe('Новая раскладка (2)')
  await layouts.open(temp)
  expect(scope).toBe(oldScope)
  expect(m.deps.center()).toEqual({ type: 'note', id: 8 })
  const before = writes.length
  await layouts.overwrite(1)
  expect(writes.length).toBe(before)
  await layouts.saveNew('Saved temporary')
  expect(typeof layouts.current).toBe('number')
  expect(db.get(Number(layouts.current))?.state.center.id).toBe(8)
  expect(layouts.items.find((i) => i.id === temp)).toBeUndefined()
  layouts.dispose()
})
test('name field filters tabs while active nonmatch stays visible once', async () => {
  const layouts = createGraphLayouts(map().deps)
  await layouts.load()
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(GraphLayouts, { target, props: { layouts } })
  flushSync()
  const input = target.querySelector('input')!
  input.value = 'not-a-layout'
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  expect(target.querySelector('.glay__outside')?.textContent).toContain('Открыт')
  expect(target.querySelectorAll('.glay__item')).toHaveLength(1)
  expect(target.querySelector('.glay__outside')?.textContent).toContain('Временная раскладка')
  input.value = ''
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  expect(target.querySelector('.glay__outside')).toBeNull()
  expect(target.children).toHaveLength(1)
  await unmount(app)
  target.remove()
  layouts.dispose()
})

test('an upload completing after a switch is added to its original board', async () => {
  const m = map(),
    layouts = createGraphLayouts(m.deps)
  await layouts.newEmpty('Upload origin')
  const origin = layouts.current
  await layouts.newEmpty('Other board')
  const current = layouts.current
  await layouts.placeEntity(origin, { type: 'attachment', id: 55, title: 'Pasted image' }, { x: 123, y: 456 })
  expect(layouts.current).toBe(current)
  expect(m.deps.center().id).toBeNull()
  expect(db.get(Number(origin))?.state.center).toEqual({ type: 'attachment', id: 55 })
  expect(db.get(Number(origin))?.state.positions['attachment:55']).toEqual({ x: 123, y: 456 })
  layouts.dispose()
})
