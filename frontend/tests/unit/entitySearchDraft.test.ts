import { expect, test } from 'bun:test'

import { draftHits, queryExtends } from '../../src/shared/lib/entitySearchDraft.ts'

const note = (id: number, title: string, snippet = '') => ({
  kind: 'note',
  id,
  title,
  where: 'body',
  snippet,
  score: 1,
})

test('a longer query keeps rows whose title or snippet contains it and adds catalog titles', () => {
  const prev = { q: 'ог', types: ['note'] }
  const next = { q: 'ого', types: ['note'] }
  expect(queryExtends(prev, next)).toBe(true)
  const rows = draftHits(prev, [note(1, 'Лечение', 'огонь в тексте'), note(2, 'Лёд', 'вода')], next, [
    { id: 3, title: 'Огонёк' },
    { id: 1, title: 'Лечение' },
    { id: 4, title: 'Тишина' },
  ])
  expect(rows.map((row) => row.id)).toEqual([1, 3])
})

test('deleting characters does not drop the wider rows', () => {
  const prev = { q: 'огонь', types: ['note'] }
  const next = { q: 'ог', types: ['note'] }
  expect(queryExtends(prev, next)).toBe(false)
  const rows = draftHits(prev, [note(1, 'Огонь')], next, [{ id: 2, title: 'Огонёк' }])
  expect(rows.map((row) => row.id)).toEqual([1, 2])
})
