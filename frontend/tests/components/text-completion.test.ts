import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getScripts: async () => [
    { id: 8, title: 'Кости', purpose: 'embed' },
    { id: 9, title: 'Хук', purpose: 'hook' },
  ],
  listValues: async () => ({ own: [], inherited: [], effective: [{ name: 'hp', type: 'int', value: 12 }] }),
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
  expect(target.querySelectorAll('[role=option]')).toHaveLength(1)
  expect(target.querySelector('[role=option]')?.textContent).toContain('Кости')
  el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
  flushSync()
  expect(el.value).toBe('script=8')
})
