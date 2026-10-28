import { expect, test } from 'bun:test'

import { MAIN_TABS, TAB_ORDER, VIEW_GROUPS, VIEW_TABS } from '../../src/shared/lib/viewTabs.ts'

test('порядок вкладок: шапка слева направо, группы сверху вниз; каждый раздел один раз', () => {
  const header = [...MAIN_TABS, ...VIEW_GROUPS.flatMap((g) => g.tabs)].map((t) => t.view)
  expect(TAB_ORDER).toEqual(header)
  expect(new Set(TAB_ORDER).size).toBe(TAB_ORDER.length)
  expect([...TAB_ORDER].sort()).toEqual(Object.keys(VIEW_TABS).sort())
})
