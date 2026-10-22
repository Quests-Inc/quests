import { expect, test } from 'bun:test'

import { effectiveText, formatValue, valueSummary } from '../../src/entities/value/model/format.ts'
import { proseValue } from '../../src/shared/lib/valueText.ts'

test('formatValue: bool is да/нет, the rest is text', () => {
  expect(proseValue('bool', false)).toBe('нет')
  expect(formatValue('bool', true)).toBe('да')
  expect(formatValue('bool', false)).toBe('нет')
  expect(formatValue('int', 3)).toBe('3')
  expect(formatValue('str', 'quests')).toBe('quests')
})

test('effectiveText and the summary line', () => {
  const text = effectiveText([
    { name: 'str', type: 'int', value: 3 },
    { name: 'flag', type: 'bool', value: true },
  ])
  expect(text).toEqual({ str: '3', flag: 'да' })
  expect(
    valueSummary([
      { name: 'str', text: '3' },
      { name: 'dc', text: '15' },
    ])
  ).toBe('str 3 · dc 15')
  expect(
    valueSummary(
      [
        { name: 'a', text: '1' },
        { name: 'b', text: '2' },
        { name: 'c', text: '3' },
      ],
      2
    )
  ).toBe('a 1 · b 2 · ещё 1')
})
