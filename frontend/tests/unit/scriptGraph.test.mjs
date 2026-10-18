import { expect, test } from 'bun:test'

import {
  addNode,
  appendGraph,
  connect,
  disconnect,
  moveNode,
  nextNodeId,
  portsKey,
  pruneEdges,
  removeNode,
  setEdge,
  setParams,
  wouldCycle,
} from '../../src/entities/script/model/graphEdit.ts'
import {
  anchorOf,
  commitPositions,
  depths,
  layout,
  nodeHeight,
  nodeKey,
  stageSize,
  viewNodes,
  wirePath,
} from '../../src/entities/script/model/graphGeometry.ts'
import { COMPARE_OPS, defaultParams, KIND_ORDER } from '../../src/entities/script/model/nodeMeta.ts'
import { assignable, portColor } from '../../src/entities/script/model/portTypes.ts'

const P = (name, type, extra = {}) => ({ name, type, ...extra })
const PORTS = {
  1: { inputs: [], outputs: [P('ref', 'ref<step>'), P('secrets', 'secrets'), P('progress', 'int'), P('title', 'str')] },
  2: {
    inputs: [P('secrets', 'secrets', { optional: true }), P('in', 'value', { optional: true, multi: true })],
    outputs: [P('stdout', 'str'), P('failed', 'str')],
  },
  3: { inputs: [P('in', 'str')], outputs: [P('progress', 'int', { optional: true }), P('bad_output', 'str')] },
  4: { inputs: [P('ref', 'ref<step>'), P('progress', 'int', { optional: true, group: 'data' })], outputs: [] },
}
const portsOf = (id) => PORTS[id]
const empty = { nodes: [], edges: [] }

test('типы портов: те же правила, что на сервере', () => {
  const yes = [
    ['int', 'int'],
    ['int', 'float'],
    ['int', 'doc'],
    ['str', 'doc'],
    ['bool', 'any'],
    ['doc', 'any'],
    ['doc', 'value'],
    ['int', 'value'],
  ]
  const no = [
    ['float', 'int'],
    ['doc', 'str'],
    ['secrets', 'any'],
    ['secrets', 'value'],
    ['ref<step>', 'ref<note>'],
    ['ref<step>', 'value'],
    ['str', 'int'],
  ]
  for (const [a, b] of yes) expect(assignable(a, b)).toBe(true)
  for (const [a, b] of no) expect(assignable(a, b)).toBe(false)
  expect(portColor('ref<note>')).toBe(portColor('ref<step>'))
  expect(portColor('int')).not.toBe(portColor('str'))
})

test('метаданные нод: порядок видов, стартовые параметры и операции сравнения', () => {
  expect(KIND_ORDER[0]).toBe('source')
  expect(KIND_ORDER.at(-1)).toBe('custom')
  expect(defaultParams('compare')).toEqual({ type: 'int', op: 'ge', operand: 1 })
  expect(defaultParams('source.step')).toBeUndefined()
  expect(COMPARE_OPS.bool.map((o) => o.id)).toEqual(['eq'])
  expect(COMPARE_OPS.str.map((o) => o.id)).toContain('matches')
})

test('граф правится значением: узел добавляется, двигается, удаляется вместе со связями', () => {
  let { graph: g, id } = addNode(empty, 'source.step', undefined, 10.4, 20.6)
  expect(id).toBe(1)
  expect(g.nodes[0]).toEqual({ id: 1, type: 'source.step', x: 10, y: 21 })
  const b = addNode(g, 'code', { text: 'true' }, 0, 0)
  g = b.graph
  expect(b.id).toBe(2)
  expect(nextNodeId(g)).toBe(3)
  expect(empty.nodes).toHaveLength(0) // исходный граф не тронут

  g = moveNode(g, 2, 100, 200)
  expect(g.nodes[1].x).toBe(100)
  g = setParams(g, 2, { text: 'false' })
  expect(g.nodes[1].params).toEqual({ text: 'false' })

  const c = connect(g, { node: 1, port: 'secrets' }, { node: 2, port: 'secrets' }, portsOf)
  expect(c.error).toBeUndefined()
  g = removeNode(c.graph, 1)
  expect(g.nodes.map((n) => n.id)).toEqual([2])
  expect(g.edges).toHaveLength(0)
})

test('связь: тип, цикл, повтор и замена на одиночном входе', () => {
  let g = { nodes: [1, 2, 3, 4].map((id) => ({ id, type: 't' })), edges: [] }
  expect(connect(g, { node: 1, port: 'title' }, { node: 3, port: 'in' }, portsOf).error).toBeUndefined() // str → str
  expect(connect(g, { node: 1, port: 'progress' }, { node: 3, port: 'in' }, portsOf).error).toContain('не подходит') // int → str
  expect(connect(g, { node: 1, port: 'secrets' }, { node: 4, port: 'ref' }, portsOf).error).toContain('не подходит')
  expect(connect(g, { node: 1, port: 'nope' }, { node: 3, port: 'in' }, portsOf).error).toBe('Такого порта нет')

  g = connect(g, { node: 1, port: 'title' }, { node: 3, port: 'in' }, portsOf).graph
  expect(connect(g, { node: 1, port: 'title' }, { node: 3, port: 'in' }, portsOf).error).toBe('Такая связь уже есть')
  // вход принимает одну связь: новая вытесняет старую
  g = connect(g, { node: 2, port: 'stdout' }, { node: 3, port: 'in' }, portsOf).graph
  expect(g.edges.map((e) => e.from.node)).toEqual([2])
  // вход «много связей» копит их
  g = connect(g, { node: 1, port: 'progress' }, { node: 2, port: 'in' }, portsOf).graph
  g = connect(g, { node: 1, port: 'title' }, { node: 2, port: 'in' }, portsOf).graph
  expect(g.edges.filter((e) => e.to.node === 2)).toHaveLength(2)

  expect(wouldCycle(g, 3, 2)).toBe(true) // 2 → 3 уже есть: 3 → 2 замкнуло бы
  expect(wouldCycle(g, 2, 3)).toBe(false)
  expect(wouldCycle(g, 2, 2)).toBe(true)
})

test('связи: убрать, задать псевдоним и необязательность, обрезать оставшиеся без портов', () => {
  let g = { nodes: [1, 2, 3].map((id) => ({ id, type: 't' })), edges: [] }
  g = connect(g, { node: 1, port: 'title' }, { node: 2, port: 'in' }, portsOf).graph
  g = connect(g, { node: 2, port: 'stdout' }, { node: 3, port: 'in' }, portsOf).graph
  g = setEdge(g, 0, { alias: 'NAME', optional: true })
  expect(g.edges[0]).toMatchObject({ alias: 'NAME', optional: true })
  g = setEdge(g, 0, { alias: '', optional: false })
  expect(g.edges[0].alias).toBeUndefined()
  expect(g.edges[0].optional).toBeUndefined()

  // у ноды 3 вход «in» пропал (например, сменили схему): связь снимается, а целая остаётся
  const shrunk = (id) => (id === 3 ? { inputs: [], outputs: [] } : PORTS[id])
  expect(pruneEdges(g, shrunk).edges.map((e) => e.to.node)).toEqual([2])
  expect(pruneEdges(g, portsOf)).toBe(g) // нечего обрезать: тот же объект
  expect(disconnect(g, 0).edges).toHaveLength(1)
})

test('ключ портов зависит от типа и параметров', () => {
  expect(portsKey({ type: 'parse', params: { a: 1 } })).toBe(portsKey({ type: 'parse', params: { a: 1 } }))
  expect(portsKey({ type: 'parse', params: { a: 1 } })).not.toBe(portsKey({ type: 'parse', params: { a: 2 } }))
  expect(portsKey({ type: 'source.step' })).toBe('source.step|')
})

test('раскладка: колонки по глубине, расставленные узлы остаются на месте', () => {
  let g = { nodes: [1, 2, 3, 4].map((id) => ({ id, type: 't' })), edges: [] }
  g = connect(g, { node: 1, port: 'secrets' }, { node: 2, port: 'secrets' }, portsOf).graph
  g = connect(g, { node: 2, port: 'stdout' }, { node: 3, port: 'in' }, portsOf).graph
  g = connect(g, { node: 1, port: 'ref' }, { node: 4, port: 'ref' }, portsOf).graph
  const d = depths(g)
  expect([d.get(1), d.get(2), d.get(3), d.get(4)]).toEqual([0, 1, 2, 1])

  g = { ...g, nodes: g.nodes.map((n) => (n.id === 4 ? { ...n, x: 7, y: 9 } : n)) }
  const laid = layout(g, portsOf)
  const at = (id) => laid.nodes.find((n) => n.id === id)
  expect(at(4)).toMatchObject({ x: 7, y: 9 })
  expect(at(1).x).toBeLessThan(at(2).x)
  expect(at(2).x).toBeLessThan(at(3).x)
  expect(layout(laid, portsOf)).toBe(laid) // всё уже стоит: тот же объект
})

test('геометрия: вид ноды (центр и размер), якоря портов, путь связи и размер сцены', () => {
  const meta = () => ({ title: 'Т', kind: 'source', color: '#7a9e3a', trusted: true })
  const g = { nodes: [{ id: 1, type: 't', x: 100, y: 50 }], edges: [] }
  const [v] = viewNodes(g, portsOf, meta)
  expect(v).toMatchObject({ key: nodeKey(1), id: 1, w: 220, h: nodeHeight(PORTS[1]) })
  expect(v.x).toBe(100 + 110) // центр = левый край + половина ширины
  expect(v.y).toBe(50 + v.h / 2)
  const out = anchorOf(v, PORTS[1], 'out', 'ref')
  expect(out).toEqual({ x: 100 + 220, y: 50 + 30 + 12 })
  expect(anchorOf(v, PORTS[1], 'out', 'progress').y).toBe(50 + 30 + 2 * 24 + 12)
  expect(anchorOf(v, PORTS[3], 'in', 'in').x).toBe(100)
  expect(nodeHeight(PORTS[1])).toBe(30 + 4 * 24 + 8)
  expect(nodeHeight(undefined)).toBe(30 + 24 + 8)
  expect(wirePath({ x: 0, y: 0 }, { x: 200, y: 100 })).toMatch(/^M 0 0 C /)
  const size = stageSize({ nodes: [{ id: 1, type: 't', x: 2000, y: 1000 }], edges: [] }, portsOf)
  expect(size.w).toBeGreaterThan(2200)
  expect(size.h).toBeGreaterThan(1100)
})

test('позиции, на которые холст утащил ноды, возвращаются в граф', () => {
  const g = {
    nodes: [
      { id: 1, type: 't', x: 0, y: 0 },
      { id: 2, type: 't', x: 500, y: 0 },
    ],
    edges: [],
  }
  const views = viewNodes(g, portsOf, () => ({ title: '', kind: 'source', color: '#000', trusted: true }))
  expect(commitPositions(g, views)).toBe(g) // не двигали: тот же объект
  views[0].x += 55.4
  views[0].y -= 10
  const moved = commitPositions(g, views)
  expect(moved.nodes[0]).toMatchObject({ x: 55, y: -10 })
  expect(moved.nodes[1]).toBe(g.nodes[1]) // нетронутая нода осталась как была
})

test('another graph is put into this one: renumbered after ours, links kept, centred on the point', () => {
  const ours = {
    nodes: [
      { id: 1, type: 'source.step', x: 0, y: 0 },
      { id: 5, type: 'sink.done', x: 10, y: 0 },
    ],
    edges: [],
  }
  const theirs = {
    nodes: [
      { id: 1, type: 'source.note', x: 0, y: 0 },
      { id: 2, type: 'sink.done', x: 200, y: 100 },
    ],
    edges: [{ from: { node: 1, port: 'ref' }, to: { node: 2, port: 'trigger' } }],
  }
  const { graph, ids } = appendGraph(ours, theirs, 500, 300)
  expect(ids).toEqual([6, 7])
  expect(graph.nodes.slice(2).map((n) => [n.id, n.x, n.y])).toEqual([
    [6, 400, 250],
    [7, 600, 350],
  ])
  expect(graph.edges).toEqual([{ from: { node: 6, port: 'ref' }, to: { node: 7, port: 'trigger' } }])
  expect(ours.nodes).toHaveLength(2)
  expect(appendGraph(ours, { nodes: [], edges: [] }, 0, 0)).toEqual({ graph: ours, ids: [] })
})
