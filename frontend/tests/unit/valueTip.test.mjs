import { expect, test } from 'bun:test'

import { parseValueChip, valueDetail, valueTypeColor, valueTypeLabel } from '../../src/shared/lib/valueTip.ts'

test('hover text is the type, and the formula when the value has one', () => {
  expect(valueTypeLabel('int')).toBe('число')
  expect(valueTypeLabel('bool')).toBe('да/нет')
  expect(valueTypeColor('int')).not.toBe(valueTypeColor('str'))
  expect(valueDetail({ formula: '(score - 10) / 2', text: '1' })).toBe('(score - 10) / 2')
  expect(valueDetail({ text: '12' })).toBe('12')
  expect(valueDetail({ error: 'деление на ноль', formula: '1 / 0', text: '' })).toBe('деление на ноль')
})

test('a chip names a local value or a foreign one', () => {
  expect(parseValueChip('str', { kind: 'note', id: 3 })).toEqual({ host: 'note:3', kind: 'note', id: 3, name: 'str' })
  expect(parseValueChip('note=4.str', { kind: 'note', id: 3 })).toEqual({
    host: 'note:4',
    kind: 'note',
    id: 4,
    name: 'str',
  })
  expect(parseValueChip('str')).toBeNull()
  expect(parseValueChip('STR', { kind: 'note', id: 1 })).toBeNull()
})
