import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { FindEntityModal } from '@/features/find-entity'

const quests = [
  { id: 1, title: 'Мир квестов', status: 'active' },
  { id: 2, title: 'Мирная жизнь', status: 'active' },
]
const notes = [{ id: 7, title: 'Мир', parent_id: null }]

function open() {
  const picked: string[] = []
  const app = mount(FindEntityModal, {
    target: document.body,
    props: {
      open: true,
      types: ['quest', 'note'],
      quests,
      notes,
      onClose: () => {},
      onPick: (t: string, id: number) => picked.push(`${t}:${id}`),
    },
  })
  flushSync()
  const input = document.querySelector('input[type=search]') as HTMLInputElement
  const type = (v: string) => {
    input.value = v
    input.dispatchEvent(new Event('input', { bubbles: true }))
    flushSync()
  }
  const key = (k: string) => {
    input.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }))
    flushSync()
  }
  const rows = () => [...document.querySelectorAll('.find__row')]
  const activeTitle = () => document.querySelector('.find__row--on .find__title')?.textContent
  return { picked, type, key, rows, activeTitle, stop: () => unmount(app) }
}

test('стрелки ходят по найденному по кругу, Enter берёт выбранное, а не первое', async () => {
  const t = open()
  t.type('мир')
  expect(t.rows().length).toBeGreaterThanOrEqual(3)
  const first = t.activeTitle()
  t.key('ArrowDown')
  const second = t.activeTitle()
  expect(second).not.toBe(first)
  t.key('ArrowUp')
  t.key('ArrowUp') // с первой строки — на последнюю
  expect(t.activeTitle()).toBe(t.rows().at(-1)?.querySelector('.find__title')?.textContent)
  t.key('ArrowDown') // и снова на первую
  expect(t.activeTitle()).toBe(first)
  t.key('ArrowDown')
  t.key('Enter')
  expect(t.picked).toHaveLength(1)
  t.stop()
})

test('подпись вида — плашка в цвете сущности, а новый поиск снова начинается с первой строки', () => {
  const t = open()
  t.type('мир')
  const kinds = [...document.querySelectorAll<HTMLElement>('.find__type')]
  expect(kinds.every((k) => k.style.getPropertyValue('--c').startsWith('#'))).toBe(true)
  expect(new Set(kinds.map((k) => k.style.getPropertyValue('--c'))).size).toBeGreaterThan(1)
  t.key('ArrowDown')
  expect(document.querySelector('.find__row--on')).not.toBe(t.rows()[0])
  t.type('ми')
  expect(document.querySelectorAll('.find__row--on')).toHaveLength(1)
  expect(document.querySelector('.find__row--on')).toBe(t.rows()[0])
  t.stop()
})
