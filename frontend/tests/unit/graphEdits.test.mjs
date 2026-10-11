import { expect, test } from 'bun:test'

import {
  diffFields,
  freshTitle,
  noteEditChanges,
  questEditFields,
  stepEditFields,
} from '../../src/entities/graph/model/edits.ts'

test('diffFields: пустое и null равны, описание сравнивается без краёв, даты — по моменту', () => {
  const orig = { title: 'A', description: ' текст ', deadline_at: '2026-01-01T00:00:00Z', pinned: false, note: null }
  const payload = {
    title: 'B',
    description: 'текст',
    deadline_at: '2026-01-01T03:00:00+03:00',
    pinned: false,
    note: '',
  }
  expect(diffFields(orig, payload, ['title', 'description', 'deadline_at', 'pinned', 'note'])).toEqual({ title: 'B' })
})

test('questEditFields: учитывает смену набора тегов независимо от порядка', () => {
  const orig = { title: 'Q', tags: [{ id: 2 }, { id: 1 }] }
  expect(questEditFields(orig, { title: 'Q', tag_ids: [1, 2] })).toEqual({})
  expect(questEditFields(orig, { title: 'Q', tag_ids: [1, 3] })).toEqual({ tag_ids: [1, 3] })
  expect(questEditFields(orig, { title: 'Новый', tag_ids: [2, 1] })).toEqual({ title: 'Новый' })
})

test('stepEditFields: только известные поля шага', () => {
  expect(stepEditFields({ title: 'S', progress_total: 1 }, { title: 'S', progress_total: 3, junk: 1 })).toEqual({
    progress_total: 3,
  })
})

test('noteEditChanges: пустой заголовок не меняет название, текст сравнивается точно', () => {
  const orig = { title: 'Заметка', description: 'один' }
  expect(noteEditChanges({ title: '  ', text: 'один', orig })).toEqual({})
  expect(noteEditChanges({ title: ' Новое ', text: 'два', orig })).toEqual({ title: 'Новое', description: 'два' })
})

test('freshTitle: первое свободное «Новая заметка (N)»', () => {
  expect(freshTitle([])).toBe('Новая заметка')
  expect(freshTitle(['Новая заметка'])).toBe('Новая заметка (2)')
  expect(freshTitle(['Новая заметка', 'Новая заметка (2)', undefined])).toBe('Новая заметка (3)')
})
