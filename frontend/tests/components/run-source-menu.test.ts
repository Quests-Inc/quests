import { afterEach, expect, mock, test } from 'bun:test'

import { mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
const filters: { source_kind?: string }[] = []
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listRuns: async (filter: { source_kind?: string }) => {
    filters.push(filter)
    return []
  },
  getScriptNodes: async () => [],
}))
const { default: RunsPage } = await import('@/pages/runs/ui/RunsPage.svelte')
let app: ReturnType<typeof mount>
let target: HTMLDivElement
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(() => {
  unmount(app)
  target.remove()
  history.replaceState({}, '', '/')
  filters.length = 0
})
test('source menu is themed, keyboard accessible and applies the selected filter', async () => {
  history.replaceState({}, '', '/?tab=runs')
  target = document.createElement('div')
  document.body.append(target)
  app = mount(RunsPage, { target })
  await tick()
  const trigger = target.querySelector<HTMLButtonElement>('[aria-label="Что запустило"]')!
  expect(target.querySelector('select')).toBeNull()
  trigger.click()
  await tick()
  const choices = [...target.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')]
  expect(choices.map((c) => c.textContent?.replace('✓', '').trim())).toContain('заметка')
  expect(document.activeElement).toBe(choices[0])
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
  expect(document.activeElement).toBe(choices[1])
  choices.find((c) => c.textContent?.trim() === 'заметка')!.click()
  await tick()
  expect(filters.at(-1)?.source_kind).toBe('note')
  expect(target.querySelector('[role="menu"]')).toBeNull()
  expect(document.activeElement).toBe(trigger)
  trigger.click()
  await tick()
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await tick()
  expect(trigger.getAttribute('aria-expanded')).toBe('false')
})
