import { expect, test } from 'bun:test'

import {
  daysLabel,
  scheduleLabel,
  parseSelectedDays,
  parseClock,
  parseLimits,
  parseWeekdays,
} from '../../src/entities/template/model/schedule.ts'

test('parseClock: часы и минуты в пределах суток, мусор — запасное значение', () => {
  expect(parseClock('7:5', '12', '00')).toEqual({ hour: '07', minute: '05' })
  expect(parseClock('99:99', '12', '00')).toEqual({ hour: '23', minute: '59' })
  expect(parseClock('', '12', '30')).toEqual({ hour: '12', minute: '30' })
  expect(parseClock(null, '08', '00')).toEqual({ hour: '08', minute: '00' })
})

test('parseWeekdays: список дней, мусор отбрасывается, пусто — будни', () => {
  expect([...parseWeekdays('0, 2,6,9,x')]).toEqual([0, 2, 6])
  expect([...parseWeekdays('')]).toEqual([0, 1, 2, 3, 4])
})

test('daysLabel: каждый день, будни, выходные, перечисление', () => {
  expect(daysLabel(new Set([0, 1, 2, 3, 4, 5, 6]))).toBe('каждый день')
  expect(daysLabel(new Set([4, 3, 2, 1, 0]))).toBe('по будням')
  expect(daysLabel(new Set([5, 6]))).toBe('по выходным')
  expect(daysLabel(new Set([0, 2])).split(', ')).toHaveLength(2)
})

test('parseLimits: только JSON-объект', () => {
  expect(parseLimits('{"max_steps": 50}')).toEqual({ max_steps: 50 })
  expect(() => parseLimits('[1]')).toThrow('Лимиты')
  expect(() => parseLimits('нет')).toThrow('Лимиты')
})

test('новые периоды: выбранные числа и подписи цикла', () => {
  expect([...parseSelectedDays('0,1,15,31,32,no', 31)]).toEqual([1, 15, 31])
  expect(scheduleLabel({ freq: 'monthly', monthdays: '31,15' })).toBe('15, 31 числа каждого месяца')
  expect(scheduleLabel({ freq: 'cycle', cycle_length: 4, cycle_days: '1,2', cycle_start: '2026-11-13' })).toBe(
    'цикл 4 дн. · дни 1, 2 · с 2026-11-13'
  )
})
