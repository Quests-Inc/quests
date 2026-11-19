import { expect, test } from 'bun:test'

import { sourceCaretFromMarkdownClick } from '@/widgets/graph-canvas/model/noteEditCaret.ts'

test('a click on rendered markdown maps into the source offset', () => {
  const root = document.createElement('div')
  root.textContent = 'hello world'
  // Mid-point of the text node approximates an offset near the space.
  const range = document.createRange()
  range.setStart(root.firstChild!, 6)
  range.collapse(true)
  const rect = range.getBoundingClientRect()
  const caret = sourceCaretFromMarkdownClick(root, 'hello world', rect.left || 1, rect.top || 1)
  expect(caret).not.toBeNull()
  expect(caret!).toBeGreaterThanOrEqual(0)
  expect(caret!).toBeLessThanOrEqual('hello world'.length)
})

test('no markdown root yields a null caret', () => {
  expect(sourceCaretFromMarkdownClick(null, 'x', 0, 0)).toBeNull()
})
