import { expect, test } from 'bun:test'

import type { Graph } from '@/shared/api/store.gen'
import { createClusterFolds } from '@/widgets/graph-canvas/model/folds.svelte.ts'
import { createGraphStage } from '@/widgets/graph-canvas/model/stage.svelte.ts'

const kinds = new Set(['questline', 'quest', 'step', 'note', 'attachment'])

function node(key: string, type: string, extra: Record<string, unknown> = {}) {
  return {
    key,
    type,
    id: Number(key.split(':')[1]) || 0,
    title: key,
    depth: 1,
    links: 0,
    sort: 0,
    step_count: 0,
    ...extra,
  }
}

function graph(): Graph {
  return {
    root: 'quest:1',
    roots: ['quest:1'],
    depth: 2,
    truncated: false,
    nodes: [
      node('quest:1', 'quest', { status: 'active', depth: 0 }),
      node('quest:2', 'quest', { status: 'done' }),
      node('step:3', 'step'),
      node('note:4', 'note'),
    ],
    edges: [
      { from: 'quest:1', to: 'quest:2', kind: 'reference' },
      { from: 'quest:2', to: 'step:3', kind: 'structure' },
      { from: 'quest:1', to: 'note:4', kind: 'reference' },
    ],
  }
}

function make() {
  const saved = new Map<string, { x: number; y: number }>()
  const folds = createClusterFolds({ saved, getNodes: () => [], getGroups: () => [] })
  let cam = { x: 0, y: 0, k: 1 }
  let active = true
  const hidden = new Set<string>()
  const prefs = {
    hidden,
    questStatuses: new Set(['active']),
    shownTypes: kinds,
    expandKeys: [] as string[],
    saveHidden(next: Set<string>) {
      prefs.hidden = next
    },
    setExpand(next: string[]) {
      prefs.expandKeys = next
    },
  }
  const data = graph()
  const el = document.createElement('div')
  el.getBoundingClientRect = () =>
    ({ width: 800, height: 600, left: 0, top: 0, right: 800, bottom: 600, x: 0, y: 0, toJSON() {} }) as DOMRect
  const stage = createGraphStage({
    getActive: () => active,
    getData: () => data,
    getDraft: () => ({ ops: [], roots: [] }),
    getDepth: () => 2,
    getQuests: () => [{ id: 2, status: 'done' }],
    prefs,
    saved,
    folds,
    initialLayout: 'tree',
    getCam: () => cam,
    setCam: (next) => {
      cam = next
    },
    clearMarquee: () => {},
    loadDraft: async () => {},
  })
  stage.el = el
  return { stage, prefs, cam: () => cam, setActive: (v: boolean) => (active = v), data }
}

async function flush() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

test('сборка убирает квест вне статуса вместе с шагом и вписывает камеру', async () => {
  const { stage, cam } = make()
  stage.compose()
  await flush()
  expect(stage.ready).toBe(true)
  expect(stage.view?.nodes.map((n) => n.key).sort()).toEqual(['note:4', 'quest:1'])
  expect(stage.filteredOut).toBe(1)
  expect(cam().k).toBeGreaterThan(0)
})

test('скрытая вкладка откладывает сборку, poke её дожимает', async () => {
  const box = make()
  box.setActive(false)
  box.stage.compose()
  await flush()
  expect(box.stage.ready).toBe(false)
  box.setActive(true)
  box.stage.poke()
  box.stage.compose()
  await flush()
  expect(box.stage.ready).toBe(true)
})

test('выделение снимается вместе с режимом, скрытие связанной карточки запоминается', async () => {
  const { stage, prefs } = make()
  stage.compose()
  await flush()
  const firstMap = stage.nodeMap
  expect(stage.nodeMap).toBe(firstMap)
  stage.toggleSelectMode()
  expect(stage.selectMode).toBe(true)
  stage.setPicked(new Set(['note:4']))
  stage.toggleSelectMode()
  expect(stage.selectMode).toBe(false)
  expect(stage.picked.size).toBe(0)
  stage.toggleSelectMode()
  await stage.hideKeys(['note:4'])
  expect([...prefs.hidden]).toEqual(['note:4'])
})

test('кружок у скрытого соседа, открытие возвращает карточку', () => {
  const { stage, prefs, data } = make()
  prefs.hidden = new Set(['note:4'])
  const marks = stage.moreMap.get('quest:1') ?? []
  expect(marks).toEqual([{ type: 'note', count: 1 }])
  const quest = data.nodes.find((n) => n.key === 'quest:1')
  stage.openMore(quest ?? { key: 'quest:1' }, 'note')
  expect(prefs.hidden.size).toBe(0)
})
