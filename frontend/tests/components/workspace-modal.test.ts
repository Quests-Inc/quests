import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
const calls: unknown[][] = []
let fail = false
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  setWorkspaceDetails: async (...args: unknown[]) => {
    if (fail) throw new Error('Нет доступа')
    calls.push(['edit', ...args])
  },
  createWorkspace: async (...args: unknown[]) => {
    calls.push(['create', ...args])
    return { id: 9 }
  },
  switchWorkspace: async (id: number) => {
    calls.push(['switch', id])
  },
}))
const { wsx, env } = await import('@/features/settings/model/workspaceState.svelte.ts')
const { default: WorkspaceModal } = await import('@/features/settings/ui/WorkspaceModal.svelte')
let app: ReturnType<typeof mount>
let target: HTMLDivElement
let closed = 0
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(async () => {
  await unmount(app)
  target.remove()
  calls.length = 0
  fail = false
})
function open(workspace: any = null) {
  closed = 0
  wsx.busy = false
  env.reload = () => {}
  target = document.createElement('div')
  document.body.append(target)
  app = mount(WorkspaceModal, {
    target,
    props: {
      open: true,
      workspace,
      onClose: () => {
        closed++
      },
    },
  })
  flushSync()
}
function submit(name: string) {
  const input = document.querySelector('.modal__form input[type=text]') as HTMLInputElement
  input.value = name
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
  document.querySelector('.modal__form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}
test('edit saves name, icon and color together; errors keep the dialog open', async () => {
  open({ id: 2, name: 'Old', color: '#6a7ab8', icon: 'map' })
  fail = true
  submit('New')
  await tick()
  flushSync()
  expect(document.querySelector('[role=alert]')?.textContent).toBe('Нет доступа')
  expect(closed).toBe(0)
  fail = false
  submit(' New ')
  await tick()
  expect(calls).toContainEqual(['edit', 2, 'New', '#6a7ab8', 'map'])
  expect(closed).toBe(1)
})
test('create includes appearance and switches to the new workspace', async () => {
  open()
  submit(' Новый мир ')
  await tick()
  expect(calls).toContainEqual(['create', 'Новый мир', { color: '#9a9a9a', icon: 'layers' }])
  expect(calls).toContainEqual(['switch', 9])
  expect(closed).toBe(1)
})
