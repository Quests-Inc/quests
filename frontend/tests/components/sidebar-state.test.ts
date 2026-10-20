import { expect, test } from 'bun:test'

import { SIDEBAR_DEFAULT, SIDEBAR_MAX, SIDEBAR_MIN, sidebars } from '@/shared/lib/sidebar.svelte.ts'

test('свёрнутость хранится по ключам независимо и переключается', () => {
  sidebars.setCollapsed('notes', true)
  expect(sidebars.isCollapsed('notes')).toBe(true)
  expect(sidebars.isCollapsed('journal')).toBe(false)
  sidebars.toggle('notes')
  expect(sidebars.isCollapsed('notes')).toBe(false)
})

test('ширина зажимается в границы и сбрасывается к значению по умолчанию', () => {
  sidebars.setWidth(10)
  expect(sidebars.width).toBe(SIDEBAR_MIN)
  sidebars.setWidth(9999)
  expect(sidebars.width).toBe(SIDEBAR_MAX)
  sidebars.setWidth(300.4)
  expect(sidebars.width).toBe(300)
  sidebars.resetWidth()
  expect(sidebars.width).toBe(SIDEBAR_DEFAULT)
})

test('ширина и свёрнутость переживают перезагрузку страницы', () => {
  sidebars.setWidth(333)
  sidebars.setCollapsed('scripts', true)
  expect(JSON.parse(localStorage.getItem('quests.sidebarWidth') ?? 'null')).toBe(333)
  expect(JSON.parse(localStorage.getItem('quests.sidebarCollapsed') ?? '{}').scripts).toBe(true)
})
