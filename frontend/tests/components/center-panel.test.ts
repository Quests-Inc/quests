import { expect, test } from 'bun:test'

import { centerPanel } from '@/shared/lib/centerPanel.ts'

test('floating panel centers and stays within its map, including after its anchor moves', () => {
  const parent = document.createElement('div'),
    panel = document.createElement('div')
  parent.append(panel)
  Object.defineProperty(parent, 'clientWidth', { value: 800 })
  parent.getBoundingClientRect = () => ({ left: 0, width: 800 }) as DOMRect
  panel.getBoundingClientRect = () => ({ width: 200 }) as DOMRect
  const action = centerPanel(panel, { center: 400 })
  try {
    expect(panel.style.left).toBe('300px')
    action.update({ center: 10 })
    expect(panel.style.left).toBe('8px')
    action.update({ center: 790 })
    expect(panel.style.left).toBe('592px')
  } finally {
    action.destroy()
  }
})
