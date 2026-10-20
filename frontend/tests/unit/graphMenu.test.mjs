import { expect, test } from 'bun:test'

import { edgeMenuItems, graphMenuItems } from '../../src/entities/graph/model/menu.ts'

const env = (extra = {}) => ({
  selectMode: false,
  pickedCount: 0,
  rootKey: 'quest:1',
  nodes: [{ key: 'quest:1', type: 'quest' }],
  expandKeys: [],
  openTarget: () => ['quest', 1],
  isExtraRoot: () => false,
  ownExpand: () => [],
  outgoingRefs: () => [],
  childKeys: () => [],
  parentNote: () => null,
  titleOf: (k) => k,
  ...extra,
})
const ctx = (extra = {}) => ({ node: null, group: false, cluster: null, edge: null, ...extra })
const ids = (items) => items.map((i) => i.id)

test('edgeMenuItems: действия по виду линии', () => {
  expect(ids(edgeMenuItems({ kind: 'reference', from: 'quest:1', to: 'note:2' }))).toEqual([
    'edge-light',
    'sep-edge',
    'edge-unref',
  ])
  expect(ids(edgeMenuItems({ kind: 'dependency', from: 'step:1', to: 'step:2' }))).toEqual([
    'edge-light',
    'sep-edge',
    'edge-nowait',
  ])
  expect(ids(edgeMenuItems({ kind: 'structure', from: 'questline:1', to: 'quest:2' }))).toEqual([
    'edge-light',
    'sep-edge',
    'edge-unline',
  ])
  expect(ids(edgeMenuItems({ kind: 'structure', from: 'questline:1', to: 'quest:t2' }))).toEqual(['edge-light'])
  expect(ids(edgeMenuItems({ kind: 'structure', from: 'note:1', to: 'note:2' }))).toEqual([
    'edge-light',
    'sep-edge',
    'edge-unparent',
  ])
  expect(edgeMenuItems({ kind: 'reference', from: 'a:1', to: 'b:2', draft: 'remove' })).toEqual([])
})

test('меню поля: создать, загрузить, добавить на карту, режим выделения', () => {
  const items = graphMenuItems(ctx(), env())
  expect(ids(items)).toEqual(['create', 'upload', 'find', 'sep-sel', 'select-mode', 'fit'])
  expect(items[0].children.map((c) => c.id)).toEqual(['new-quest', 'new-note', 'new-step', 'new-questline'])
  expect(graphMenuItems(ctx(), env({ selectMode: true })).at(-2).label).toBe('Выйти из режима выделения')
})

test('меню выделения, шара и линии', () => {
  expect(graphMenuItems(ctx({ group: true }), env({ pickedCount: 3 }))).toEqual([
    { id: 'hide-picked', label: 'Убрать с карты (3)' },
    { id: 'clear-picked', label: 'Снять выделение' },
  ])
  expect(ids(graphMenuItems(ctx({ cluster: { folded: true, key: null } }), env()))).toEqual(['cl-open', 'cl-disband'])
  expect(ids(graphMenuItems(ctx({ cluster: { folded: false, key: 'cluster:x' } }), env()))).toEqual([
    'cl-fold',
    'cl-disband',
  ])
  expect(ids(graphMenuItems(ctx({ edge: { kind: 'reference', from: 'a:1', to: 'b:2' } }), env()))).toEqual([
    'edge-light',
    'sep-edge',
    'edge-unref',
  ])
})

test('меню карточки: центр, правка, ссылки, подписи', () => {
  const quest = { key: 'quest:7', type: 'quest' }
  const items = graphMenuItems(
    ctx({ node: quest }),
    env({ outgoingRefs: () => ['note:3'], childKeys: () => ['step:1'] })
  )
  expect(ids(items)).toEqual(
    expect.arrayContaining([
      'open',
      'center',
      'hide',
      'edit',
      'copy',
      'add',
      'link-to',
      'link-from',
      'unlink',
      'expand',
      'drop-steps',
    ])
  )
  expect(items.find((i) => i.id === 'unlink').children[0].label).toBe('→ note:3')
  // корень карты нельзя сделать центром ещё раз
  const root = graphMenuItems(ctx({ node: { key: 'quest:1', type: 'quest' } }), env())
  expect(ids(root)).not.toContain('center')
})

test('меню карточки черновика: можно убрать из черновика, но не «открыть»', () => {
  const items = graphMenuItems(ctx({ node: { key: 'note:t3', type: 'note', draft: true, op: 3 } }), env())
  expect(ids(items)).toContain('drop')
  expect(ids(items)).not.toContain('open')
  expect(items.at(-1)).toMatchObject({ id: 'drop', danger: true })
  const note = graphMenuItems(ctx({ node: { key: 'note:2', type: 'note' } }), env({ parentNote: () => 'note:1' }))
  expect(note.find((i) => i.id === 'detach-note').label).toBe('Открепить от «note:1»')
})

test('edgeMenuItems: у уже подсвеченной линии пункт — «Снять подсветку»', () => {
  const e = { kind: 'reference', from: 'quest:1', to: 'note:2' }
  expect(edgeMenuItems(e)[0].label).toBe('Подсветить узлы')
  expect(edgeMenuItems(e, { ...e })[0].label).toBe('Снять подсветку')
  expect(edgeMenuItems(e, { kind: 'reference', from: 'quest:1', to: 'note:3' })[0].label).toBe('Подсветить узлы')
})
