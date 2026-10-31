import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
const writes: unknown[][] = []
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listValues: async () => ({ own: [{ name: 'runner', type: 'str', value: 'pc' }] }),
  getRunners: async () => [
    { label: 'pc', registered: true, online: true },
    { label: 'laptop', registered: true, online: false },
    { label: 'revoked', registered: true, revoked: true },
    { label: 'unknown', registered: false },
  ],
  putValue: async (...args: unknown[]) => {
    writes.push(args)
  },
}))
const { default: DefaultRunner } = await import('@/features/settings/ui/sections/DefaultRunner.svelte')
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
test('default runner uses registered choices and preserves ordered fallback runners', async () => {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(DefaultRunner, { target })
  try {
    await tick()
    flushSync()
    expect(target.querySelector('input[type=text]')).toBeNull()
    const select = target.querySelector('select')!
    expect([...select.options].map((o) => o.value)).toEqual(['', 'laptop'])
    expect(select.textContent).toContain('офлайн')
    select.value = 'laptop'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    flushSync()
    target.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await tick()
    expect(writes[0]).toEqual(['workspace', 0, 'runner', 'str', 'pc,laptop'])
  } finally {
    await unmount(app)
    target.remove()
  }
})
