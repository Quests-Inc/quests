import { expect, test } from 'bun:test'

import { prepareGraph } from '../../src/entities/graph/model/prepare.ts'

function graphWithFiles(count) {
  const nodes = [{ key: 'note:1', type: 'note', id: 1, depth: 0, links: 0 }]
  const edges = []
  for (let i = 0; i < count; i++) {
    nodes.push({ key: `attachment:${i}`, type: 'attachment', id: i, depth: 1, links: 0 })
    edges.push({ from: 'note:1', to: `attachment:${i}`, kind: 'structure' })
  }
  return { nodes, edges }
}

test('prepareGraph: мало вложений остаются карточками', () => {
  const out = prepareGraph(graphWithFiles(3), new Set(), new Set())
  expect(out.nodes).toHaveLength(4)
  expect(out.edges).toHaveLength(3)
})

test('prepareGraph: много вложений сворачиваются в один узел-кластер', () => {
  const out = prepareGraph(graphWithFiles(8), new Set(), new Set())
  expect(out.nodes.map((n) => n.key)).toEqual(['note:1', 'cluster:note:1'])
  expect(out.nodes[1]).toMatchObject({ type: 'cluster', title: '8 вложений', depth: 1 })
  expect(out.edges).toEqual([{ from: 'note:1', to: 'cluster:note:1', kind: 'structure' }])
})

test('prepareGraph: раскрытый кластер и «россыпь» не сворачиваются', () => {
  expect(prepareGraph(graphWithFiles(8), new Set(['cluster:note:1']), new Set()).nodes).toHaveLength(9)
  expect(prepareGraph(graphWithFiles(8), new Set(), new Set(['note:1'])).nodes).toHaveLength(9)
})
