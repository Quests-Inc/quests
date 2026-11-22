import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import GraphCanvas from '@/widgets/graph-canvas/ui/GraphCanvas.svelte'

function mountScene({ existing = true }: { existing?: boolean } = {}) {
  const node = {
    key: 'attachment:42',
    id: 42,
    type: 'attachment',
    title: 'Image.png',
    mime: 'image/png',
    x: 100,
    y: 100,
    w: 96,
    h: 96,
    links: 0,
    depth: 0,
  }
  const nodes = existing ? [node] : []
  const view = existing ? { nodes, edges: [], groups: [] } : null
  const stage = {
    el: null,
    view,
    nodes,
    edges: [],
    groups: [],
    drawEdges: [],
    nodeMap: new Map(nodes.map((n) => [n.key, n])),
    stageSize: { w: 800, h: 600 },
    ready: existing,
    previewing: false,
    sceneHidden: false,
    full: false,
    selectMode: false,
    picked: new Set(),
    selectedKey: null,
    selected: null,
    lit: null,
    front: {},
    ownerless: new Set(),
    touched: new Map(),
    moreMap: new Map(),
    mini: null,
    canRecenter: false,
    clearPicked() {},
    toggleFull() {},
    fit() {},
    relayout() {},
    focusZone() {},
    measureContent() {},
    openTarget() {
      return null
    },
    openMore() {},
  }
  const gest = {
    cam: { x: 0, y: 0, k: 1 },
    moving: false,
    marquee: null,
    onTouchDown() {},
    onBgDown() {},
    onMove() {},
    onUp() {},
    onWheel() {},
    onCardDown() {},
    onGroupDown() {},
    worldAt() {
      return { x: 0, y: 0 }
    },
  }
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(GraphCanvas, {
    target,
    props: {
      stage,
      gest,
      folds: { collapse() {} },
      lk: { linking: null, canPick: () => true, cursor: null },
      data: existing ? { root: node.key } : null,
      center: { id: existing ? 42 : null },
      error: '',
      loading: true,
      canClear: false,
      tier: new Map(),
      edgeActions: () => [],
      onCtx() {},
      onGroupCtx() {},
      onEdgeCtx() {},
      onStartEdit() {},
      onFinishEdit() {},
      onShowChanges() {},
      onHidePicked() {},
      onToggleSelect() {},
      onClear() {},
      onRecenter() {},
    } as any,
  })
  flushSync()
  return { app, target }
}

test('background graph loading keeps the existing scene visible', async () => {
  const { app, target } = mountScene()
  expect(target.querySelector('.graph__msg')).toBeNull()
  expect(target.querySelector('.gworld')?.classList.contains('gworld--hidden')).toBe(false)
  expect(target.querySelector('[data-gkey="attachment:42"]')).not.toBeNull()
  expect(target.querySelector('.graph__foot')).not.toBeNull()
  await unmount(app)
  target.remove()
})

test('initial graph loading still replaces an absent scene with status', async () => {
  const { app, target } = mountScene({ existing: false })
  expect(target.querySelector('.graph__msg')?.textContent).toContain('Загрузка карты')
  expect(target.querySelector('.gworld')?.classList.contains('gworld--hidden')).toBe(true)
  await unmount(app)
  target.remove()
})
