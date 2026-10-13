import { expect, test } from 'bun:test'

import { buildRefLabels } from '../../src/app/model/refLabels.ts'
import { inScope, listOpenOrAll, scopeQuestlineChoices } from '../../src/entities/quest/model/journalView.ts'

const quests = [
  { id: 1, status: 'active', category_id: 1, questline_id: 10 },
  { id: 2, status: 'completed', category_id: 1, questline_id: 11 },
  { id: 3, status: 'frozen', category_id: 2, questline_id: null },
]

test('listOpenOrAll: по умолчанию только открытые, «все» или явный статус — без отсева', () => {
  expect(listOpenOrAll(quests, { showAll: false, statusTokens: [] }).map((q) => q.id)).toEqual([1])
  expect(listOpenOrAll(quests, { showAll: true, statusTokens: [] })).toHaveLength(3)
  expect(listOpenOrAll(quests, { showAll: false, statusTokens: ['frozen'] })).toHaveLength(3)
})

test('inScope: раздел и квестлайн сужают список независимо', () => {
  expect(inScope(quests, { categoryId: 1, questlineId: null }).map((q) => q.id)).toEqual([1, 2])
  expect(inScope(quests, { categoryId: 1, questlineId: 11 }).map((q) => q.id)).toEqual([2])
  expect(inScope(quests, { categoryId: null, questlineId: null })).toHaveLength(3)
})

test('scopeQuestlineChoices: без «все» — только квестлайны с открытым квестом', () => {
  const lines = [
    { id: 10, category_id: 1 },
    { id: 11, category_id: null },
    { id: 12, category_id: 1 },
  ]
  expect(scopeQuestlineChoices({ scopeCategoryId: null, questlines: lines, quests, showAll: true })).toEqual([])
  expect(
    scopeQuestlineChoices({ scopeCategoryId: 1, questlines: lines, quests, showAll: true }).map((l) => l.id)
  ).toEqual([10, 11, 12])
  expect(
    scopeQuestlineChoices({ scopeCategoryId: 1, questlines: lines, quests, showAll: false }).map((l) => l.id)
  ).toEqual([10])
})

test('«без раздела»: inScope и варианты квестлайнов', () => {
  const qs = [
    { id: 1, status: 'active', category_id: null, questline_id: 20 },
    { id: 2, status: 'active', category_id: 1, questline_id: 10 },
    { id: 3, status: 'active', category_id: null, questline_id: null },
  ]
  expect(inScope(qs, { categoryId: 'none', questlineId: null }).map((q) => q.id)).toEqual([1, 3])
  expect(inScope(qs, { categoryId: 'none', questlineId: 20 }).map((q) => q.id)).toEqual([1])
  const lines = [
    { id: 10, category_id: 1 },
    { id: 20, category_id: null },
    { id: 21, category_id: null },
  ]
  expect(
    scopeQuestlineChoices({ scopeCategoryId: 'none', questlines: lines, quests: qs, showAll: true }).map((l) => l.id)
  ).toEqual([20, 21])
  expect(
    scopeQuestlineChoices({ scopeCategoryId: 'none', questlines: lines, quests: qs, showAll: false }).map((l) => l.id)
  ).toEqual([20])
})

test('buildRefLabels: подписи ссылок всех видов, пустое название — kind=id', () => {
  const labels = buildRefLabels({
    notes: [{ id: 1, title: 'Заметка' }],
    quests: [{ id: 2, title: '', steps: [{ id: 3, title: 'Шаг' }] }],
    questlines: [{ id: 4, title: 'Линия' }],
    templates: [{ id: 5, title: 'Шаблон' }],
    attachments: [{ id: 6, filename: 'a.pdf' }],
  })
  expect(labels).toEqual({
    'note:1': 'Заметка',
    'quest:2': 'quest=2',
    'step:3': 'Шаг',
    'questline:4': 'Линия',
    'template:5': 'Шаблон',
    'attachment:6': 'a.pdf',
  })
})
