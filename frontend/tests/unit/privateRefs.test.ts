import { expect, test } from 'bun:test'

import { referenceEdges } from '@/entities/graph/model/privateRefs.ts'

test('a revealed note links only to cards already on the map', () => {
  const nodes = [{ key: 'note:4' }, { key: 'note:9' }]
  expect(referenceEdges(nodes, [], [['note:4', 'см. note=9 и note=4']])).toEqual([
    { from: 'note:4', to: 'note:9', kind: 'reference' },
  ])
  expect(referenceEdges(nodes, [], [['note:4', 'note=8']])).toEqual([])
})
