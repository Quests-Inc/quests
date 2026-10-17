import { expect, test } from 'bun:test'

import {
  checkPayload,
  formatProgressRange,
  mirrorQuestId,
  parseProgressRange,
  questStepDraft,
  questStepsPayload,
  templateStepDraft,
  templateStepsPayload,
} from '../../src/entities/quest/model/steps.ts'
import { applyMention, activeMentionToken, matchMentions } from '../../src/shared/lib/mentionSuggest.ts'
import { buildSuggestIndex, nearestVote, pickSuggestion, suggestTags } from '../../src/shared/lib/suggest.ts'

test('диапазон прогресса: разбор «5», «5..10», «5-10», перепутанные границы, формат', () => {
  expect(parseProgressRange('')).toEqual({ progress_min: 1, progress_max: 1 })
  expect(parseProgressRange('5')).toEqual({ progress_min: 5, progress_max: 5 })
  expect(parseProgressRange('5..10')).toEqual({ progress_min: 5, progress_max: 10 })
  expect(parseProgressRange('10-5')).toEqual({ progress_min: 5, progress_max: 10 })
  expect(parseProgressRange('0')).toEqual({ progress_min: 1, progress_max: 1 })
  expect(formatProgressRange(3, 3)).toBe('3')
  expect(formatProgressRange(2, 7)).toBe('2..7')
  expect(formatProgressRange(0, 0)).toBe('1')
})

test('checkPayload: без команды проверка выключена целиком; интервал не меньше 15 с', () => {
  expect(checkPayload({ check_command: '  ', wait_previous: true, run_mode: 'watch', run_on: ' Home ' })).toEqual({
    check_command: null,
    script_id: null,
    check_interval_seconds: null,
    wait_previous: false,
    run_mode: 'poll',
    run_on: '',
  })
  expect(
    checkPayload({
      check_command: ' ls ',
      check_interval_seconds: '5',
      wait_previous: 1,
      run_mode: 'watch',
      run_on: ' Home ',
    })
  ).toEqual({
    check_command: 'ls',
    script_id: null,
    check_interval_seconds: 15,
    wait_previous: true,
    run_mode: 'watch',
    run_on: 'home',
  })
  expect(checkPayload({ check_command: 'ls', check_interval_seconds: '', run_mode: 'bogus' })).toMatchObject({
    check_interval_seconds: null,
    run_mode: 'poll',
  })
})

test('черновики и данные для API: безымянные шаги отбрасываются, порядок и ограничения', () => {
  const blank = questStepDraft()
  expect(blank).toMatchObject({ id: null, title: '', progress_current: 0, progress_total: 1, open: false })
  expect(blank.key).toBeTruthy()
  expect(questStepDraft({ id: 4, title: 'Т', description: 'Д', check_command: 'x', run_mode: 'once' })).toMatchObject({
    key: '4',
    open: true,
    run_mode: 'once',
  })
  expect(
    questStepsPayload([
      { id: null, title: ' Первый ', description: ' о ', progress_current: -2, progress_total: 0 },
      { id: null, title: '  ', description: '', progress_current: 0, progress_total: 1 },
      { id: 7, title: 'Третий', description: '', progress_current: 2, progress_total: 5 },
    ]).map((s) => [s.id, s.title, s.description, s.progress_current, s.progress_total, s.sort_order])
  ).toEqual([
    [null, 'Первый', 'о', 0, 1, 0],
    [7, 'Третий', '', 2, 5, 2],
  ])
  expect(templateStepDraft(null).progress_range).toBe('1')
  expect(templateStepDraft({ id: 1, title: 'Ш', progress_min: 2, progress_max: 4 }).progress_range).toBe('2..4')
  expect(
    templateStepsPayload([
      { title: 'Ш', progress_range: '3-5' },
      { title: '', progress_range: '1' },
    ])
  ).toMatchObject([{ title: 'Ш', progress_min: 3, progress_max: 5, sort_order: 0 }])
})

test('mirrorQuestId: первый quest=N в текстах шага', () => {
  expect(mirrorQuestId('сделать note=3', 'и quest=9, потом quest=10')).toBe(9)
  expect(mirrorQuestId('ничего', '')).toBeNull()
})

test('matchMentions: начало названия выше «содержит», шаги несут квест, дубли вложения схлопываются, лимит', () => {
  const src = {
    notes: [
      { id: 1, title: 'Хлеб' },
      { id: 2, title: 'Про хлеб' },
    ],
    quests: [{ id: 5, title: 'Купить', steps: [{ id: 9, title: 'хлебный шаг' }] }],
    attachments: [{ id: 3, filename: 'хлеб.pdf', comment: 'про хлеб' }],
    questlines: [],
    templates: [],
  }
  const r = matchMentions('хлеб', src)
  expect(r.map((x) => `${x.kind}=${x.id}`)).toEqual(['note=1', 'attachment=3', 'step=9', 'note=2'])
  expect(r.find((x) => x.kind === 'step').hint).toBe('Купить')
  expect(matchMentions('хлеб', src, 2)).toHaveLength(2)
  expect(matchMentions('   ', src)).toEqual([])
})

test('@-упоминание: токен под кареткой и подстановка kind=id', () => {
  expect(activeMentionToken('привет @хле', 11)).toEqual({ query: 'хле', start: 7 })
  expect(activeMentionToken('привет @хле ', 12)).toBeNull()
  expect(activeMentionToken('почта a@b', 3)).toBeNull()
  const token = activeMentionToken('см @хле дальше', 7)
  expect(applyMention('см @хле дальше', token, { kind: 'note', id: 12 })).toEqual({
    text: 'см note=12  дальше',
    caret: 11,
  })
})

test('предложения: ближайшие по тексту голосуют за квестлайн, теги и порог показа', () => {
  const index = buildSuggestIndex([
    {
      title: 'Оплатить интернет',
      description: 'квитанция за месяц',
      questline_id: 1,
      category_id: 1,
      tags: [{ id: 1, slug: 'money', label: 'Деньги' }],
    },
    {
      title: 'Оплатить электричество',
      description: 'квитанция',
      questline_id: 1,
      category_id: 1,
      tags: [{ id: 1, slug: 'money', label: 'Деньги' }],
    },
    { title: 'Почистить кроссовки', description: 'щётка и крем', questline_id: 2, category_id: 2, tags: [] },
    { title: 'Сходить к стоматологу', description: 'запись', questline_id: null, category_id: 3, tags: [] },
  ])
  const vote = nearestVote(index, { title: 'Оплатить воду', description: 'квитанция' }, 'questline')
  expect(vote.id).toBe(1)
  expect(vote.share).toBeGreaterThan(0.7)
  expect(pickSuggestion(vote)).toBe(1)
  expect(pickSuggestion({ id: 1, share: 0.6, sim: 0.2 }, 1)).toBe(1)
  expect(pickSuggestion({ id: 1, share: 0.6, sim: 0.2 }, null)).toBeNull()
  expect(pickSuggestion(null)).toBeNull()
  expect(suggestTags(index, { title: 'Оплатить воду', description: 'квитанция' }, { excludeSlugs: ['MONEY'] })).toEqual(
    []
  )
  expect(suggestTags(index, { title: 'Оплатить воду', description: 'квитанция' })[0].tag.slug).toBe('money')
  expect(nearestVote(buildSuggestIndex([]), { title: 'x' }, 'questline')).toBeNull()
})

test('checkPayload: шаг со скриптом сохраняет расписание и устройство, а id скрипта уходит числом', () => {
  expect(checkPayload({ script_id: '7', check_interval_seconds: '60', run_mode: 'once', run_on: 'Home' })).toEqual({
    check_command: null,
    script_id: 7,
    check_interval_seconds: 60,
    wait_previous: false,
    run_mode: 'once',
    run_on: 'home',
  })
})
