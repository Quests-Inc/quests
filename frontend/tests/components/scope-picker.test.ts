import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import ScopePicker from '@/shared/ui/ScopePicker.svelte'

const categories = [
  { id: 1, label: 'Работа', color: '#5a8a9a' },
  { id: 2, label: 'Дом' },
]
const many = Array.from({ length: 12 }, (_, i) => ({ id: 100 + i, title: `Линия ${i}` }))
const lines: Record<string, { id: number; title: string }[]> = {
  '1': [
    { id: 10, title: 'Проект' },
    { id: 11, title: 'Рутина' },
  ],
  '2': [],
  none: many,
}

function setup(category: number | 'none' | null = null, questline: number | null = null) {
  const picks: [number | 'none' | null, number | null][] = []
  const app = mount(ScopePicker, {
    target: document.body,
    props: {
      categories,
      category,
      questline,
      linesFor: (c: number | 'none') => lines[String(c)] ?? [],
      onPick: (c: number | 'none' | null, l: number | null) => picks.push([c, l]),
    },
  })
  const btn = () => document.body.querySelector('.scope__btn') as HTMLButtonElement
  const rows = () => [...document.body.querySelectorAll('.scope__pop:not(.scope__sub) [data-row]')] as HTMLElement[]
  const sub = () => document.body.querySelector('.scope__sub') as HTMLElement | null
  const hover = (el: HTMLElement) => {
    el.dispatchEvent(new Event('pointerenter'))
    flushSync()
  }
  return { picks, btn, rows, sub, hover, done: () => unmount(app) }
}

test('кнопка показывает выбор; меню: все разделы, разделы, «Без раздела»', () => {
  const t = setup(1, 11)
  expect(t.btn().textContent).toContain('Работа › Рутина')
  t.btn().click()
  flushSync()
  expect(t.rows().map((r) => r.textContent?.trim().replace('✓', '').trim())).toEqual([
    'Все разделы',
    'Работа',
    'Дом',
    'Без раздела',
  ])
  t.done()
})

test('наведение на раздел открывает квестлайны; только категория и категория+квестлайн выбираются', () => {
  const t = setup()
  t.btn().click()
  flushSync()
  expect(t.sub()).toBeNull()
  t.hover(t.rows()[1])
  expect(t.sub()).not.toBeNull()
  const items = [...t.sub()!.querySelectorAll('.scope__row')].map((e) => e.textContent?.trim())
  expect(items).toEqual(['Весь раздел', 'Проект', 'Рутина'])
  ;(t.sub()!.querySelectorAll('.scope__row')[2] as HTMLElement).click()
  expect(t.picks.at(-1)).toEqual([1, 11])
  t.btn().click()
  flushSync()
  t.rows()[1].click()
  expect(t.picks.at(-1)).toEqual([1, null])
  t.done()
})

test('«Все разделы» и раздел без квестлайнов не открывают боковое меню', () => {
  const t = setup()
  t.btn().click()
  flushSync()
  t.hover(t.rows()[0])
  expect(t.sub()).toBeNull()
  t.hover(t.rows()[2])
  expect(t.sub()).toBeNull()
  t.rows()[0].click()
  expect(t.picks.at(-1)).toEqual([null, null])
  t.done()
})

test('«Без раздела» — тоже раздел: длинный список получает поиск и фильтр', () => {
  const t = setup()
  t.btn().click()
  flushSync()
  t.hover(t.rows()[3])
  const search = t.sub()!.querySelector('.scope__search') as HTMLInputElement
  expect(search).not.toBeNull()
  search.value = 'Линия 11'
  search.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  expect([...t.sub()!.querySelectorAll('.scope__list .scope__row')].map((e) => e.textContent?.trim())).toEqual([
    'Линия 11',
  ])
  ;(t.sub()!.querySelector('.scope__list .scope__row') as HTMLElement).click()
  expect(t.picks.at(-1)).toEqual(['none', 111])
  t.done()
})

test('короткий список квестлайнов обходится без поиска', () => {
  const t = setup()
  t.btn().click()
  flushSync()
  t.hover(t.rows()[1])
  expect(t.sub()!.querySelector('.scope__search')).toBeNull()
  t.done()
})
