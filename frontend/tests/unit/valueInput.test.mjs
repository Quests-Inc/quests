import { expect, test } from 'bun:test'

import { readValueInput, valueInputText } from '../../src/shared/lib/valueInput.ts'

test('str: quotes make a value, an operator makes a formula, plain text is a value', () => {
  expect(readValueInput('str', 'Волк')).toEqual({ value: 'Волк', formula: '' })
  expect(readValueInput('str', 'name + " волка"')).toEqual({ value: '', formula: 'name + " волка"' })
  expect(readValueInput('str', 'hp < 10')).toEqual({ value: '', formula: 'hp < 10' })
  expect(readValueInput('str', '"a + b"')).toEqual({ value: 'a + b', formula: '' })
  expect(readValueInput('str', '"1/4"')).toEqual({ value: '1/4', formula: '' })
  expect(readValueInput('str', '1/4')).toEqual({ value: '1/4', formula: '' })
})

test('numbers and bool: a constant is a value, anything else a formula', () => {
  expect(readValueInput('int', ' 12 ')).toEqual({ value: 12, formula: '' })
  expect(readValueInput('int', '(str - 10) / 2')).toEqual({ value: '', formula: '(str - 10) / 2' })
  expect(readValueInput('float', '0.5')).toEqual({ value: 0.5, formula: '' })
  expect(readValueInput('int', '')).toEqual({ error: 'нужно значение' })
  expect(readValueInput('bool', 'Да')).toEqual({ value: true, formula: '' })
  expect(readValueInput('bool', 'hp < 10')).toEqual({ value: '', formula: 'hp < 10' })
})

test('a stored row reads back as the same row', () => {
  for (const value of ['a + b', '"q"', 'plain', '1/4']) {
    const read = readValueInput('str', valueInputText({ type: 'str', value }))
    expect(read).toEqual({ value, formula: '' })
  }
  expect(valueInputText({ type: 'int', value: 0, formula: '(str - 10) / 2' })).toBe('(str - 10) / 2')
  expect(valueInputText({ type: 'str', value: 'a + b' })).toBe('"a + b"')
  expect(valueInputText({ type: 'bool', value: true })).toBe('да')
})
