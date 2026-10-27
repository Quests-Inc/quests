import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
const old = { nodes: [{ id: 1, type: 'args' }], edges: [] }
const current = { nodes: [], edges: [] }
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  getScriptVersion: async (_id: number, version: number) => {
    return { id: 5, version, graph: version === 1 ? old : current }
  },
}))
const { default: VersionsModal } = await import('@/pages/scripts/ui/VersionsModal.svelte')
let app: ReturnType<typeof mount>, target: HTMLDivElement
const tick = () => new Promise((r) => setTimeout(r, 0))
afterEach(() => {
  if (app) unmount(app)
  target?.remove()
})
test('версия сравнивается с редактором; откат требует подтверждения и передаёт старую схему', async () => {
  target = document.createElement('div')
  document.body.append(target)
  const restored: unknown[] = []
  app = mount(VersionsModal, {
    target,
    props: {
      open: true,
      id: 5,
      version: 2,
      graph: current,
      dirty: true,
      onClose() {},
      onRestore: async (graph) => {
        restored.push(graph)
      },
    },
  })
  await tick()
  const select = target.querySelector('select')!
  select.selectedIndex = 1
  select.dispatchEvent(new Event('change', { bubbles: true }))
  flushSync()
  await tick()
  expect(target.textContent).toContain('Версия 1')
  ;[...target.querySelectorAll('button')].find((b) => b.textContent?.includes('Откатить к версии'))!.click()
  flushSync()
  expect(target.textContent).toContain('Несохранённые правки редактора будут заменены')
  expect(restored).toHaveLength(0)
  const confirmation = [...target.querySelectorAll('.backdrop')].find((el) =>
    el.textContent?.includes('Откатить скрипт?')
  ) as HTMLElement
  expect(confirmation.style.zIndex).toBe('90')
  ;[...confirmation.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Откатить')!.click()
  await tick()
  expect(restored).toEqual([old])
})
