import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import type { LayoutDeps, LayoutState } from '@/pages/graph/model/layouts.svelte.ts'
import GraphLayouts from '@/pages/graph/ui/GraphLayouts.svelte'

const writes: { id: number; state?: LayoutState }[] = []
mock.module('@/shared/api/graph.ts', () => ({
  listGraphLayouts: async () => [],
  getGraphLayout: async () => ({
    id: 7,
    title: 'Работа',
    state: { entities: [{ key: 'note:2', title: 'Старая заметка' }] },
  }),
  updateGraphLayout: async (id: number, patch: { state?: LayoutState }) => {
    writes.push({ id, ...patch })
    return { id, title: 'Работа' }
  },
  createGraphLayout: async () => ({}),
  deleteGraphLayout: async () => ({}),
}))
const { createGraphLayouts } = await import('@/pages/graph/model/layouts.svelte.ts')

test('перезапись сначала показывает разницу, отмена не пишет, подтверждение сохраняет просмотренный снимок', async () => {
  const nodes = [{ key: 'quest:1', title: 'Квест', x: 10, y: 20 }]
  const deps: LayoutDeps = {
    center: () => ({ type: 'quest', id: 1 }),
    setCenter: () => {},
    clearCenter: async () => {},
    roots: () => [],
    writeRoots: async () => {},
    loadDraft: async () => {},
    depth: { get: () => 2, set: () => {} },
    layout: { get: () => 'tree', set: () => {} },
    prefs: {
      questStatuses: new Set(['active']),
      shownTypes: new Set(['quest']),
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
  const layouts = createGraphLayouts(deps)
  await layouts.overwrite(7)
  expect(writes).toHaveLength(0)
  expect(layouts.overwriteAsk?.diff).toEqual({
    added: [{ key: 'quest:1', title: 'Квест' }],
    removed: [{ key: 'note:2', title: 'Старая заметка' }],
  })
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(GraphLayouts, { target, props: { layouts } })
  flushSync()
  // SidebarLayout needs a single root even while a dialog is open, including on mobile.
  expect(target.children).toHaveLength(1)
  expect(target.querySelector('aside [role="dialog"]')).not.toBeNull()
  await unmount(app)
  target.remove()
  layouts.cancelOverwrite()
  expect(layouts.overwriteAsk).toBeNull()
  expect(writes).toHaveLength(0)
  await layouts.overwrite(7)
  nodes[0].x = 500
  await layouts.confirmOverwrite()
  expect(writes).toHaveLength(1)
  expect(writes[0].state?.positions['quest:1'].x).toBe(10)
  expect(layouts.overwriteAsk).toBeNull()
})
