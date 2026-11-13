import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import PeriodPicker from '@/entities/template/ui/PeriodPicker.svelte'

let clean: (() => void)[] = []
afterEach(() => {
  for (const fn of clean) fn()
  clean = []
})
function render(freq: string) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(PeriodPicker, {
    target,
    props: {
      freq,
      weekdays: new Set([0, 1]),
      monthdays: new Set([15]),
      cycleLength: 4,
      cycleStart: '2026-11-13',
      cycleDays: new Set([1, 2]),
    },
  })
  clean.push(() => {
    unmount(app)
    target.remove()
  })
  flushSync()
  return target
}
test('месяц: 31 число и независимый множественный выбор', () => {
  const root = render('monthly')
  const days = root.querySelectorAll('.period-day')
  expect(days).toHaveLength(31)
  expect(days[14].getAttribute('aria-pressed')).toBe('true')
  flushSync(() => (days[30] as HTMLButtonElement).click())
  expect(days[30].getAttribute('aria-pressed')).toBe('true')
  expect(days[14].getAttribute('aria-pressed')).toBe('true')
})
test('свой цикл: редактируемая длина и отдельные дни 2/2', () => {
  const root = render('cycle')
  expect(root.querySelectorAll('.period-day')).toHaveLength(4)
  const selected = () =>
    [...root.querySelectorAll('.period-day[aria-pressed="true"]')].map((d) => d.textContent?.trim())
  expect(selected()).toEqual(['1', '2'])
  const input = root.querySelector('input[type=number]') as HTMLInputElement
  flushSync(() => {
    input.value = '6'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  expect(root.querySelectorAll('.period-day')).toHaveLength(6)
  expect(selected()).toEqual(['1', '2'])
})
