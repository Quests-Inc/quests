import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
let role = 'owner'
const writes: unknown[] = []
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listValues: async () => ({ own: [], inherited: [], effective: [] }),
  listSecrets: async () => ({
    keys: ['TOKEN'],
    own: [{ key: 'TOKEN', from: 'note', id: 7 }],
    inherited: [{ key: 'PARENT', from: 'note', id: 6, title: 'Родитель' }],
    effective: [],
  }),
  fetchWorkspaces: async () => ({ current: 1, workspaces: [{ id: 1, role }] }),
  setSecret: async (...args: unknown[]) => {
    writes.push(args)
  },
}))
const { ValueTable } = await import('@/entities/value')
let app: ReturnType<typeof mount> | undefined
let target: HTMLDivElement
const tick = () => new Promise((r) => setTimeout(r, 0))
afterEach(() => {
  if (app) unmount(app)
  target?.remove()
  writes.length = 0
  role = 'owner'
})
async function open() {
  target = document.createElement('div')
  document.body.append(target)
  app = mount(ValueTable, { target, props: { variant: 'panel', owner: { kind: 'note', id: 7 } } })
  await tick()
  ;[...target.querySelectorAll('button')].find((b) => b.textContent === 'Секреты')!.click()
  await tick()
}
test('окружение: вкладка секретов показывает источник и принимает новое значение без отправки формы', async () => {
  await open()
  expect(target.textContent).toContain('Родитель')
  expect(target.textContent).toContain('TOKEN')
  const key = target.querySelector('[aria-label="Имя секрета"]') as HTMLInputElement
  const value = target.querySelector('[aria-label="Новое значение секрета"]') as HTMLInputElement
  key.value = 'NEW'
  key.dispatchEvent(new Event('input', { bubbles: true }))
  value.value = 'sensitive'
  value.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  value.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
  await tick()
  expect(writes).toEqual([['note', 7, 'NEW', 'sensitive']])
  expect(value.value).toBe('')
  expect(target.textContent).not.toContain('sensitive')
})
test('участник видит имена, но не может записать секрет', async () => {
  role = 'member'
  await open()
  expect(target.textContent).toContain('TOKEN')
  expect(target.querySelector('[aria-label="Новое значение секрета"]')).toBeNull()
  expect(target.querySelector('[aria-label="Удалить TOKEN"]')).toBeNull()
})
