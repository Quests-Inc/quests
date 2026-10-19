import { expect, test } from 'bun:test'

import { rowMeta, streakLabel, templateMenuItems } from '../../src/features/templates/model/rowInfo.ts'

test('streakLabel: пусто без закрытых, иначе серия и процент', () => {
  expect(streakLabel(undefined)).toBe('')
  expect(streakLabel({ closed: 0 })).toBe('')
  expect(streakLabel({ closed: 3, close_rate: 0.5, current_streak: 2, longest_streak: 4 })).toBe(
    'серия 2 · лучшая 4 · 50%'
  )
})

test('rowMeta: расписание, режим, флаги и число шагов', () => {
  const s = rowMeta(
    {
      freq: 'daily',
      emit_mode: 'surprise',
      emit_pool_command: 'x',
      pinned: true,
      steps: [{}, {}],
      significance: 'nope',
    },
    { closed: 1, close_rate: 1, current_streak: 1, longest_streak: 1 }
  )
  expect(s).toBe('каждый день · случайно · команда · обычное · закреплён · серия 1 · лучшая 1 · 100% · 2 шага')
  expect(rowMeta({ freq: 'daily', steps: [] })).toContain('по расписанию')
})

test('templateMenuItems: пусто без шаблона; подпись включения зависит от enabled', () => {
  expect(templateMenuItems(null)).toEqual([])
  expect(templateMenuItems({ id: 7, enabled: true }).find((i) => i.id === 'toggle').label).toBe('Выключить')
  expect(templateMenuItems({ id: 7 }).find((i) => i.id === 'toggle').label).toBe('Включить')
  expect(templateMenuItems({ id: 7 })[0].label).toBe('Копировать template=7')
})

test('templateMenuItems: проверка выпуска только у шаблона со скриптом или командой', () => {
  expect(templateMenuItems({ id: 7 }).some((i) => i.id === 'emit-test')).toBe(false)
  expect(templateMenuItems({ id: 7, script_id: 13 }).some((i) => i.id === 'emit-test')).toBe(true)
  expect(templateMenuItems({ id: 7, emit_pool_command: 'echo' }).some((i) => i.id === 'emit-test')).toBe(true)
})
