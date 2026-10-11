import { expect, test } from 'bun:test'

import { applyDraft, describeOp } from '../../src/entities/graph/model/graphDraft.ts'
import { layoutLayers } from '../../src/entities/graph/model/graphLayout.ts'

const graph = () => ({
  nodes: [
    { key: 'quest:1', type: 'quest', id: 1, title: 'Квест', depth: 0, links: 0 },
    { key: 'note:2', type: 'note', id: 2, title: 'Заметка', depth: 1, links: 0 },
  ],
  edges: [],
})

test('applyDraft: create_quest добавляет черновой узел и не трогает исходный граф', () => {
  const g = graph()
  const out = applyDraft(g, [{ id: 5, kind: 'create_quest', payload: { fields: { title: 'Новый' } } }])
  expect(g.nodes).toHaveLength(2)
  expect(out.nodes).toHaveLength(3)
  expect(out.nodes[2]).toMatchObject({ key: 'quest:t5', title: 'Новый', draft: true, status: 'active' })
})

test('applyDraft: add_ref рисует ссылку, remove_ref помечает существующую как удаляемую', () => {
  const g = graph()
  const added = applyDraft(g, [{ id: 1, kind: 'add_ref', payload: { from: 'quest:1', to: 'note:2' } }])
  expect(added.edges).toEqual([{ from: 'quest:1', to: 'note:2', kind: 'reference', draft: 'add', op: 1 }])
  expect(added.touched.get('quest:1')).toEqual([1])

  g.edges.push({ from: 'quest:1', to: 'note:2', kind: 'reference' })
  const removed = applyDraft(g, [{ id: 2, kind: 'remove_ref', payload: { from: 'quest:1', to: 'note:2' } }])
  expect(removed.edges[0]).toMatchObject({ draft: 'remove', op: 2 })
})

test('describeOp: одна строка журнала на операцию', () => {
  const title = (k) => `<${k}>`
  expect(describeOp({ kind: 'create_quest', payload: { fields: { title: 'А' } } }, title)).toBe('Создать квест «А»')
  expect(describeOp({ kind: 'add_ref', payload: { from: 'a', to: 'b' } }, title)).toBe('Ссылка: «<a>» → «<b>»')
  expect(describeOp({ kind: 'edit_quest', payload: { id: 'quest:1', fields: { questline_id: null } } }, title)).toBe(
    'Вывести квест «<quest:1>» из квестлайна'
  )
})

test('layoutLayers: колонки по типам слева направо, внутри — сверху вниз', () => {
  const nodes = [
    { type: 'note', title: 'Б', depth: 1, w: 100, h: 40 },
    { type: 'quest', title: 'А', depth: 0, w: 100, h: 40 },
    { type: 'quest', title: 'В', depth: 1, w: 100, h: 40 },
  ]
  layoutLayers(nodes)
  const [note, q1, q2] = nodes
  expect(q1.x).toBe(q2.x)
  expect(q1.y).toBeLessThan(q2.y)
  expect(note.x).toBeGreaterThan(q1.x)
})
