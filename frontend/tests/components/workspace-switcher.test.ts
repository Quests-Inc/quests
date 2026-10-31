import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
const calls: string[] = []
let fail = false
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  fetchWorkspaces: async () => ({
    current: 1,
    workspaces: [
      { id: 1, name: 'Мир', color: '#6a7ab8', icon: 'map', role: 'owner' },
      { id: 2, name: 'Работа', color: '#7a9e3a', icon: 'target', role: 'member' },
    ],
  }),
  switchWorkspace: async (id: number) => {
    calls.push(`switch:${id}`)
    if (fail) throw new Error('Нет доступа')
  },
  setWorkspaceAppearance: async (id: number, color: string, icon: string) => {
    calls.push(`appearance:${id}:${color}:${icon}`)
  },
}))
const { default: Switcher } = await import('@/features/settings/ui/WorkspaceSwitcher.svelte')
const { wsx, env, saveWorkspaceAppearance } = await import('@/features/settings/model/workspaceState.svelte.ts')
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))

test('header loads workspaces, shows current name and switches via the shared operation', async () => {
  wsx.loaded = false
  wsx.busy = false
  let reloads = 0
  env.reload = () => {
    reloads++
  }
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(Switcher, { target })
  try {
    flushSync()
    await tick()
    flushSync()
    const trigger = target.querySelector<HTMLButtonElement>('.workspace-trigger')!
    expect(trigger.textContent).toContain('Мир')
    trigger.click()
    flushSync()
    const options = () => [...target.querySelectorAll<HTMLButtonElement>('[role=menuitemradio]')]
    expect(options()).toHaveLength(2)
    expect(options()[0].getAttribute('aria-checked')).toBe('true')
    options()[1].click()
    await tick()
    flushSync()
    expect(calls).toContain('switch:2')
    expect(reloads).toBe(1)
    fail = true
    trigger.click()
    flushSync()
    options()[1].click()
    await tick()
    flushSync()
    expect(target.querySelector('[role=alert]')?.textContent).toBe('Нет доступа')
    expect(trigger.textContent).toContain('Мир')
    expect(reloads).toBe(1)
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    flushSync()
    expect(target.querySelector('[role=menu]')).toBeNull()
    fail = false
    await saveWorkspaceAppearance('#c47a20', 'flag')
    flushSync()
    expect(target.querySelector('.workspace-switcher')?.getAttribute('style')).toContain('#c47a20')
    expect(calls).toContain('appearance:1:#c47a20:flag')
  } finally {
    await unmount(app)
    target.remove()
  }
})
