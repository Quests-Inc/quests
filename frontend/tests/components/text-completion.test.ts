import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { ENTITY_COLORS } from '@/shared/lib/entityColors.ts'
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getScripts: async () => [
    { id: 8, title: 'Кости', purpose: 'embed' },
    { id: 10, title: 'Другой расчёт', purpose: 'embed' },
    { id: 9, title: 'Хук', purpose: 'hook' },
  ],
  listValues: async () => ({ own: [], inherited: [], effective: [{ name: 'hp', type: 'int', value: 12 }] }),
}))
mock.module('@/shared/api/entitySearch.ts', () => ({
  searchEntities: async (q: { q: string; types: string[] }) => ({
    hits: [
      { id: 8, title: 'Кости', kind: 'script' },
      { id: 10, title: 'Другой расчёт', kind: 'script' },
      { id: 1, title: 'Заметка', kind: 'note' },
    ].filter((x) => q.types.includes(x.kind) && x.title.toLowerCase().includes(q.q.toLowerCase())),
    more: false,
    next: 3,
  }),
}))
const { MentionTextarea } = await import('@/entities/template')
let app: ReturnType<typeof mount>, target: HTMLDivElement
const tick = () => new Promise((r) => setTimeout(r, 0))
afterEach(() => {
  if (app) unmount(app)
  target?.remove()
})
async function type(value: string) {
  const el = target.querySelector('textarea')!
  el.value = value
  el.setSelectionRange(value.length, value.length)
  el.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  await tick()
  return el
}
test('редактор дополняет видимую переменную и предлагает только скрипты-вставки', async () => {
  target = document.createElement('div')
  document.body.append(target)
  app = mount(MentionTextarea, { target, props: { host: { kind: 'note', id: 3 } } })
  const el = await type('{{h')
  expect(target.querySelector('[role=option]')?.textContent).toContain('hp')
  el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
  flushSync()
  expect(el.value).toBe('{{hp}}')
  await type('script=')
  expect(target.querySelectorAll('[role=option]')).toHaveLength(2)
  expect(target.querySelector('[role=option]')?.textContent).toContain('Кости')
  el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
  flushSync()
  expect(el.value).toBe('script=8')
})

test('@ opens without query, uses the graph palette, and selected script opens insertion modal', async () => {
  target = document.createElement('div')
  document.body.append(target)
  app = mount(MentionTextarea, { target, props: { notes: [{ id: 1, title: 'Заметка' }] } })
  expect(target.querySelector('.insert-script')).toBeNull()
  const el = await type('@')
  expect(target.querySelectorAll('[role=option]')).toHaveLength(3)
  const note = [...target.querySelectorAll('[role=option]')].find((e) => e.textContent?.includes('Заметка'))!
  expect((note.querySelector('.mentions__kind') as HTMLElement).style.color).toBe(ENTITY_COLORS.note)
  await type('до @Друг')
  ;(target.querySelector('[role=option]') as HTMLElement).click()
  flushSync()
  await tick()
  flushSync()
  expect(document.querySelector('[role=dialog]')).not.toBeNull()
  expect(document.querySelector<HTMLSelectElement>('[role=dialog] select')?.value).toBe('10')
  const input = document.querySelector<HTMLInputElement>('input[placeholder="2к6 + сила"]')!
  input.value = '1+2'
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  document.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  flushSync()
  await tick()
  expect(el.value).toBe('до script=10{1+2}')
  expect(document.querySelector('[role=dialog]')).toBeNull()
})
