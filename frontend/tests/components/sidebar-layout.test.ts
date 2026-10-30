import { expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { sidebars } from '@/shared/lib/sidebar.svelte.ts'

import SidebarLayoutHarness from './SidebarLayoutHarness.svelte'

test('свёрнутая панель не рисует и не перехватывает элементы; ручка снова раскрывает панель', async () => {
  sidebars.setCollapsed('sidebar-regression', false)
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(SidebarLayoutHarness, { target })
  flushSync()
  const panel = target.querySelector('aside') as HTMLElement
  const handle = target.querySelector('.sbl__handle') as HTMLButtonElement
  expect(handle.getAttribute('aria-label')).toBe('Скрыть боковую панель')
  handle.click()
  flushSync()
  expect(sidebars.isCollapsed('sidebar-regression')).toBe(true)
  expect(getComputedStyle(panel).visibility).toBe('hidden')
  expect(getComputedStyle(panel).pointerEvents).toBe('none')
  expect(handle.getAttribute('aria-label')).toBe('Показать боковую панель')
  handle.click()
  flushSync()
  expect(sidebars.isCollapsed('sidebar-regression')).toBe(false)
  expect(getComputedStyle(panel).visibility).not.toBe('hidden')
  expect(target.querySelector('[role="separator"]')).not.toBeNull()
  await unmount(app)
  target.remove()
})
