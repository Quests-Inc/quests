import { expect, test } from 'bun:test'

import { DEFAULT_ICON_COLOR, TINT_SHARE, tintOf, tinted } from '../../src/entities/graph/model/tint.ts'

const node = (type, extra = {}) => ({ key: `${type}:1`, type, title: 'x', ...extra })

test('квест, заметка и квестлайн: свой цвет, цвет по умолчанию оттенка не даёт', () => {
  for (const type of ['quest', 'note', 'questline']) {
    expect(tintOf(node(type, { color: '#7a9e3a' }))).toBe('#7a9e3a')
    expect(tintOf(node(type, { color: DEFAULT_ICON_COLOR.toUpperCase() }))).toBe('')
    expect(tintOf(node(type))).toBe('')
  }
})

test('шаг берёт цвет своего квеста, без квеста или с цветом по умолчанию — без оттенка', () => {
  expect(tintOf(node('step'), { color: '#b54a3a' })).toBe('#b54a3a')
  expect(tintOf(node('step'), { color: DEFAULT_ICON_COLOR })).toBe('')
  expect(tintOf(node('step'), null)).toBe('')
})

test('вложение — цвет типа файла, прочие типы файлов и другие сущности — без оттенка', () => {
  expect(tintOf(node('attachment', { title: 'схема.png', mime: 'image/png' }))).toContain('--ft-image')
  expect(tintOf(node('attachment', { title: 'blob.xyz', mime: 'application/octet-stream' }))).toBe('')
  expect(tintOf(node('template'))).toBe('')
})

test('tinted: цвет типа сохраняет большую часть, оттенок — только долю', () => {
  expect(tinted('#c9a227', '')).toBe('#c9a227')
  const mixed = tinted('#c9a227', '#7a9e3a')
  expect(mixed).toBe(`color-mix(in srgb, #c9a227 ${100 - TINT_SHARE}%, #7a9e3a ${TINT_SHARE}%)`)
  expect(TINT_SHARE).toBeLessThanOrEqual(35)
})
