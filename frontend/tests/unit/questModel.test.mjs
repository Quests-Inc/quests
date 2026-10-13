import { expect, test } from 'bun:test'

import {
  OPEN_STATUSES,
  isQuestInactive,
  periodBadge,
  questTimer,
  quantifiedProgress,
  significanceLabel,
} from '../../src/entities/quest/model/questFormat.ts'

test('статусы: открытые и неактивные', () => {
  expect([...OPEN_STATUSES]).toEqual(['active', 'expired'])
  expect(isQuestInactive({ status: 'completed' })).toBe(true)
  expect(isQuestInactive({ status: 'active' })).toBe(false)
})

test('подписи: период шаблона, значимость, прогресс только у многошаговых', () => {
  expect(periodBadge({ template_id: null, period_key: '2026-01-01' })).toBeNull()
  expect(periodBadge({ template_id: 1, period_key: '2026-01-01' })).toBe('2026-01-01')
  expect(periodBadge({ template_id: 1, period_key: '' })).toBe('цикл')
  expect(significanceLabel({ significance: 'epic' })).toBe('эпическое')
  expect(significanceLabel({ significance: '' })).toBe('обычное')
  expect(significanceLabel({ significance: 'новое' })).toBe('новое')
  expect(quantifiedProgress({ steps_total: 1, progress_label: '1/1' })).toBeNull()
  expect(quantifiedProgress({ steps_total: 4, progress_label: '1/4' })).toBe('1/4')
})

test('questTimer: нет срока, закрытый или просроченный — без таймера; окно задаёт цвет', () => {
  const now = Date.parse('2026-03-01T10:00:00Z')
  expect(questTimer({}, now)).toBeNull()
  expect(questTimer({ deadline_at: '2026-03-01T11:00:00Z', status: 'completed' }, now)).toBeNull()
  expect(questTimer({ deadline_at: '2026-03-01T09:00:00Z', status: 'active' }, now)).toBeNull()
  const t = questTimer({ deadline_at: '2026-03-01T10:30:00Z', status: 'active', duration_seconds: 3600 }, now)
  expect(t).toMatchObject({ rem: 1800, label: '30:00', tone: 'orange', detailLabel: '30:00 / 1:00:00' })
  expect(questTimer({ deadline_at: '2026-03-01T10:30:00Z', status: 'active' }, now).tone).toBeNull()
})

import { KINDS, detailOf } from '../../src/entities/history/model/historyKinds.ts'
import { findEntities } from '../../src/entities/quest/model/entityFlags.ts'
import { questSpec } from '../../src/entities/quest/model/questFlags.ts'
import { groupQuestsByCategory, partitionByQuestline } from '../../src/entities/quest/model/questGroups.ts'
import { parseQuery, passes } from '../../src/shared/lib/searchFlags.ts'

test('groupQuestsByCategory: разделы по порядку, затем неизвестные, «без раздела» в конце; внутри — квестлайны', () => {
  const cats = [
    { id: 2, label: 'Б', sort_order: 1, color: '#111' },
    { id: 1, label: 'А', sort_order: 0 },
  ]
  const lines = [{ id: 7, title: 'Линия', category_id: 1, color: '#222' }]
  const quests = [
    { id: 1, category_id: 2, title: 'q1' },
    { id: 2, category_id: 1, questline_id: 7, created_at: '2026-01-02', title: 'q2' },
    { id: 3, category_id: 1, questline_id: 7, created_at: '2026-01-01', title: 'q3' },
    { id: 4, category_id: 9, category_label: 'Старый', title: 'q4' },
    { id: 5, category_id: null, title: 'q5' },
  ]
  const out = groupQuestsByCategory(quests, cats, lines)
  expect(out.map((g) => g.key)).toEqual(['c1', 'c2', 'c9', 'none'])
  expect(out[0]).toMatchObject({ label: 'А', color: '#9a9a9a', questCount: 2 })
  expect(out[0].lines[0]).toMatchObject({ title: 'Линия', color: '#222' })
  expect(out[0].lines[0].quests.map((q) => q.id)).toEqual([3, 2])
  expect(out[2].label).toBe('Старый')
  expect(out[3]).toMatchObject({ label: 'Без раздела', color: null })
})

test('partitionByQuestline: свободные квесты отдельно, линии по алфавиту', () => {
  const r = partitionByQuestline(
    [{ id: 2, questline_id: 2, questline_title: 'Я' }, { id: 1, questline_id: 1, questline_title: 'А' }, { id: 3 }],
    []
  )
  expect(r.lines.map((l) => l.title)).toEqual(['А', 'Я'])
  expect(r.alone.map((q) => q.id)).toEqual([3])
  expect(r.questCount).toBe(3)
})

test('questSpec: фильтры поиска по статусу, флагам, виду, значимости, тегам, разделу и квестлайну', () => {
  const spec = questSpec({ categories: [{ id: 1, slug: 'work', label: 'Работа' }] })
  const quest = {
    status: 'completed',
    pinned: true,
    automated: false,
    significance: 'Epic',
    tags: [{ slug: 'infra', label: 'Инфра' }],
    category_slug: 'work',
    questline_id: 5,
    questline_title: 'Моя линия',
  }
  const ok = (q) => passes(parseQuery(q, spec), spec, quest)
  expect(ok('status:done')).toBe(true)
  expect(ok('status:active')).toBe(false)
  expect(ok('status:all')).toBe(true)
  expect(ok('flag:pinned')).toBe(true)
  expect(ok('flag:hidden')).toBe(false)
  expect(ok('type:manual')).toBe(true)
  expect(ok('sig:ep')).toBe(true)
  expect(ok('tag:infra')).toBe(true)
  expect(ok('tag:нет')).toBe(false)
  expect(ok('cat:work')).toBe(true)
  expect(ok('cat:none')).toBe(false)
  expect(ok('line:5')).toBe(true)
  expect(ok('line:моя_линия')).toBe(true)
})

test('findEntities: пустой запрос ничего не ищет; поиск по названию и по kind=id; фильтр типа и лимит', () => {
  const data = {
    quests: [
      { id: 1, title: 'Купить хлеб' },
      { id: 2, title: 'Хлебные крошки' },
    ],
    notes: [{ id: 1, title: 'Хлеб', outdated: true }],
    questlines: [{ id: 3, title: 'Быт' }],
  }
  expect(findEntities('', data).items).toEqual([])
  expect(findEntities('хлеб', data).items.map((i) => `${i.type}=${i.id}`)).toEqual(['quest=1', 'quest=2', 'note=1'])
  expect(findEntities('quest=2', data).items.map((i) => i.id)).toEqual([2])
  expect(findEntities('type:note хлеб', data).items.map((i) => i.type)).toEqual(['note'])
  expect(findEntities('flag:outdated', data).items.map((i) => i.type)).toEqual(['note'])
  expect(findEntities('хлеб', { ...data, limit: 1 }).items).toHaveLength(1)
})

test('historyKinds: подписи видов и значимая подробность', () => {
  expect(KINDS.quest_completed).toEqual(['квест выполнен', 'checkmark', 'ok'])
  expect(detailOf({ detail: '  изменено ', title: 'x' })).toBe('')
  expect(detailOf({ detail: 'Период 2026-01-01', title: 'x' })).toBe('Период 2026-01-01')
  expect(detailOf({ detail: 'x', title: 'x' })).toBe('')
  expect(detailOf({})).toBe('')
})

test('questSpec: cat:none — квесты без раздела', () => {
  const spec = questSpec({ categories: [{ id: 1, slug: 'work', label: 'Работа' }] })
  const free = { status: 'active', category_slug: null }
  const work = { status: 'active', category_slug: 'work' }
  expect(passes(parseQuery('cat:none', spec), spec, free)).toBe(true)
  expect(passes(parseQuery('cat:none', spec), spec, work)).toBe(false)
  expect(spec.cat.values.some((v) => v.id === 'none')).toBe(true)
})
