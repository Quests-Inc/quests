import { expect, test } from 'bun:test'

import { PURPOSES, purposeOfKind } from '../../src/entities/script/model/purpose.ts'

test('a picker kind maps to the purpose of the scripts it offers', () => {
  expect(purposeOfKind('step')).toBe('step')
  expect(purposeOfKind('quest')).toBe('hook')
  expect(purposeOfKind('workspace')).toBe('hook')
  for (const k of ['step', 'note', 'template']) expect(PURPOSES.some((p) => p.id === purposeOfKind(k))).toBe(true)
})
