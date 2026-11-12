import { expect, test } from 'bun:test'

import { entityMatches, identityMatches, rankIdentity, searchIdentity } from '../../src/shared/lib/entitySearch.ts'

test('identity is exact and typed, while a numeric text query retains ordinary matches', () => {
  expect(searchIdentity(' Note = 378 ')).toEqual({ kind: 'note', id: '378' })
  expect(identityMatches('378', 'note', 378)).toBe(true)
  expect(identityMatches('378', 'note', 1378)).toBe(false)
  expect(entityMatches('note=378', 'quest', 378, 'note=378')).toBe(false)
  expect(entityMatches('378', 'note', 10, 'версия 378')).toBe(true)
  expect(searchIdentity('note=0')).toBeNull()
  expect(
    rankIdentity(
      [
        { kind: 'note', id: 2 },
        { kind: 'note', id: 378 },
      ],
      '378',
      (x) => x
    ).map((x) => x.id)
  ).toEqual([378, 2])
})
