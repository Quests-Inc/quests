import { expect, test } from 'bun:test'

import type { GraphNodeView } from '@/entities/graph'
import { createGraphMenu } from '@/pages/graph/model/menu.svelte.ts'

const node = (key: string, type: string, id: number | string = 1): GraphNodeView => ({
  key,
  type,
  id,
  title: key,
  links: 0,
  depth: 0,
  x: 10,
  y: 20,
  w: 100,
  h: 40,
})

function make() {
  const calls: string[] = []
  const menu = createGraphMenu({
    worldAt: () => ({ x: 3, y: 4 }),
    isLinking: () => false,
    cancelLink: () => calls.push('cancel'),
    selectMode: () => false,
    pickedHas: () => false,
    pickedSize: () => 0,
    setSelectedKey: (key) => calls.push(`sel:${key}`),
    select: (n) => calls.push(`open:${n.key}`),
    collapseKey: () => 'cluster:quest:1',
    getDraftRoots: () => [{ type: 'note', id: 2 }],
    getRootKey: () => 'quest:1',
    getNodes: () => [node('quest:1', 'quest')],
    getExpandKeys: () => [],
    openTarget: () => ['quest', 1],
    ownExpand: () => [],
    outgoingRefs: () => [],
    childKeys: () => ['step:9'],
    parentNote: () => null,
    titleOf: (key) => key,
    hidePicked: async () => calls.push('hide-picked'),
    toggleSelectMode: () => calls.push('mode'),
    fit: () => calls.push('fit'),
    copyRef: (n) => calls.push(`ref:${n.type}=${n.id}`),
    highlightEdge: (e) => calls.push(`light:${e.from}>${e.to}`),
    clearPicked: () => calls.push('clear-picked'),
    hideKeys: async (keys) => calls.push(`hide:${keys.join(',')}`),
    createNoteHere: async (_at, from) => calls.push(`note:${from}`),
    openEntity: (kind) => calls.push(`entity:${kind}`),
    startEdit: async (n) => calls.push(`edit:${n.key}`),
    addOp: async (kind) => calls.push(`op:${kind}`),
    askDrop: async () => calls.push('drop'),
    startLink: (mode) => calls.push(`link:${mode}`),
    collapse: (key) => calls.push(`fold:${key}`),
    disband: (owner) => calls.push(`disband:${owner}`),
    unlinkRef: async () => calls.push('unref'),
    setExpand: (keys) => calls.push(`expand:${keys.join(',')}`),
    loadDraft: async () => calls.push('draft'),
    putRoots: async (roots) => calls.push(`roots:${roots.length}`),
    onOpen: (kind, id) => calls.push(`go:${kind}:${id}`),
    recenter: () => calls.push('recenter'),
    saved: new Map(),
  })
  const ev = (extra: Record<string, unknown> = {}) =>
    ({
      clientX: 11,
      clientY: 22,
      preventDefault() {},
      stopPropagation() {},
      ...extra,
    }) as MouseEvent
  return { menu, calls, ev }
}

test('правый клик по карточке выделяет её и открывает меню', () => {
  const { menu, calls, ev } = make()
  menu.onCtx(ev(), node('note:2', 'note', 2))
  expect(menu.ctx.open).toBe(true)
  expect(menu.ctx.node?.key).toBe('note:2')
  expect(calls).toEqual(['sel:note:2'])
  expect(menu.items().some((item) => item.id === 'open' || item.children)).toBe(true)
})

test('шаг «Развернуть» раскрывает шар, «Убрать шаги» прячет детей', async () => {
  const { menu, calls, ev } = make()
  menu.onCtx(ev(), node('cluster:quest:1', 'cluster', 0))
  expect(menu.ctx.cluster?.folded).toBe(true)
  await menu.onCtxSelect('cl-open')
  expect(calls).toEqual(['open:cluster:quest:1'])
  menu.onCtx(ev(), node('quest:1', 'quest', 1))
  await menu.onCtxSelect('drop-steps')
  expect(calls).toContain('hide:step:9')
})

test('создание квеста открывает форму, закрытие её прячет', async () => {
  const { menu, ev } = make()
  menu.onCtx(ev(), null)
  await menu.onCtxSelect('new-quest')
  expect(menu.questModal.open).toBe(true)
  expect(menu.questModal.at).toEqual({ x: 3, y: 4 })
  menu.closeQuest()
  expect(menu.questModal.open).toBe(false)
  menu.close()
  expect(menu.ctx.open).toBe(false)
})

test('меню пустого поля: режим выделения и «Вписать в экран» — и они срабатывают', async () => {
  const { menu, calls, ev } = make()
  menu.onCtx(ev(), null as never)
  const ids = menu.items().map((item) => item.id)
  expect(ids).toContain('select-mode')
  expect(ids).toContain('fit')
  await menu.onCtxSelect('fit')
  expect(calls).toContain('fit')
})

test('«Подсветить узлы» в меню линии зовёт подсветку именно этой линии', async () => {
  const { menu, calls, ev } = make()
  menu.onEdgeCtx(ev(), { kind: 'reference', from: 'note:1', to: 'note:2' })
  expect(menu.items().map((i) => i.id)[0]).toBe('edge-light')
  await menu.onCtxSelect('edge-light')
  expect(calls).toEqual(['light:note:1>note:2'])
})

test('меню карточки: «Копировать quest=N» первым, ссылка на сущность уходит в буфер', async () => {
  const { menu, calls, ev } = make()
  menu.onCtx(ev(), node('quest:5', 'quest', 5))
  expect(menu.items()[0].label).toBe('Копировать quest=5')
  await menu.onCtxSelect('copy-ref')
  expect(calls).toContain('ref:quest=5')
})
