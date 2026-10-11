import { expect, test } from 'bun:test'

import {
  canPickTarget,
  childKeysOf,
  hasRefEdge,
  noteDescendantKeys,
  outgoingRefKeys,
  parentNoteOf,
} from '../../src/entities/graph/model/links.ts'

const nodes = new Map(
  [
    ['quest:1', 'quest'],
    ['note:2', 'note'],
    ['note:3', 'note'],
    ['note:4', 'note'],
    ['step:5', 'step'],
    ['attachment:6', 'attachment'],
    ['cluster:note:2', 'cluster'],
  ].map(([key, type]) => [key, { key, type }])
)
const edges = [
  { from: 'note:2', to: 'note:3', kind: 'structure' },
  { from: 'note:3', to: 'note:4', kind: 'structure' },
  { from: 'quest:1', to: 'step:5', kind: 'structure' },
  { from: 'quest:1', to: 'step:t9', kind: 'structure' },
  { from: 'quest:1', to: 'note:2', kind: 'reference' },
  { from: 'quest:1', to: 'note:4', kind: 'reference', draft: 'remove' },
]

test('noteDescendantKeys: вся ветка заметок, без удаляемых связей', () => {
  expect([...noteDescendantKeys(edges, 'note:2')].sort()).toEqual(['note:3', 'note:4'])
  expect([...noteDescendantKeys(edges, 'note:4')]).toEqual([])
})

test('canPickTarget: ссылка «на» и «от» — по видам, кластер и сам якорь нельзя', () => {
  const to = { mode: 'to', anchor: 'quest:1' }
  expect(canPickTarget(to, nodes.get('attachment:6'), nodes, edges)).toBe(true)
  expect(canPickTarget(to, nodes.get('quest:1'), nodes, edges)).toBe(false)
  expect(canPickTarget(to, nodes.get('cluster:note:2'), nodes, edges)).toBe(false)
  expect(canPickTarget({ mode: 'from', anchor: 'quest:1' }, nodes.get('attachment:6'), nodes, edges)).toBe(false)
  expect(canPickTarget(null, nodes.get('note:2'), nodes, edges)).toBe(false)
})

test('canPickTarget: заметку нельзя вложить в своего потомка; шаг — только в квест', () => {
  const move = { mode: 'attach', anchor: 'note:2' }
  expect(canPickTarget(move, nodes.get('note:4'), nodes, edges)).toBe(false)
  expect(canPickTarget({ mode: 'attach', anchor: 'note:4' }, nodes.get('note:2'), nodes, edges)).toBe(true)
  expect(canPickTarget({ mode: 'attach', anchor: 'step:5' }, nodes.get('quest:1'), nodes, edges)).toBe(true)
  expect(canPickTarget({ mode: 'attach', anchor: 'step:5' }, nodes.get('note:2'), nodes, edges)).toBe(false)
})

test('childKeysOf, parentNoteOf, hasRefEdge, outgoingRefKeys', () => {
  expect(childKeysOf(edges, nodes, 'quest:1', 'step')).toEqual(['step:5'])
  expect(parentNoteOf(edges, 'note:3')).toBe('note:2')
  expect(parentNoteOf(edges, 'note:2')).toBeNull()
  expect(hasRefEdge(edges, 'quest:1', 'note:2')).toBe(true)
  expect(hasRefEdge(edges, 'quest:1', 'note:4')).toBe(false)
  expect(outgoingRefKeys(edges, nodes, 'quest:1')).toEqual(['note:2'])
})
