import { expect, test } from 'bun:test'

import { valueHover } from '@/shared/lib/valueHover.ts'
import { hostKey, publishValues, readValue, touchValues, valueRev } from '@/shared/lib/valueLive.svelte.ts'

test('published values are what a chip reads back', () => {
  publishValues(hostKey('note', 3), [
    { name: 'str', type: 'int', text: '1', formula: '(str_score - 10) / 2', error: '' },
  ])
  expect(readValue('note:3', 'str')?.formula).toBe('(str_score - 10) / 2')
  expect(readValue('note:3', 'dex')).toBeUndefined()
  const before = valueRev.n
  touchValues('note:3')
  expect(valueRev.n).toBe(before + 1)
  expect(valueRev.host).toBe('note:3')
})

test('hover waits, then shows type, name and formula; a right click asks to edit', async () => {
  publishValues('note:3', [{ name: 'str', type: 'int', text: '1', formula: '(str_score - 10) / 2', error: '' }])
  document.body.innerHTML = '<div id="host"><span class="md-value" data-value="str">1</span></div>'
  const root = document.getElementById('host')!
  let edited = ''
  const action = valueHover(
    root,
    () => ({ host: { kind: 'note', id: 3 }, onEdit: (target) => (edited = target.name) }),
    20
  )
  root.querySelector('.md-value')!.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }))
  expect(document.querySelector('.md-value-tip')).toBeNull()
  await new Promise((resolve) => setTimeout(resolve, 40))
  const tip = document.querySelector('.md-value-tip')
  expect(tip?.textContent).toContain('число')
  expect(tip?.textContent).toContain('str')
  expect(tip?.textContent).toContain('(str_score - 10) / 2')
  root.querySelector('.md-value')!.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  expect(edited).toBe('str')
  expect(document.querySelector('.md-value-tip')).toBeNull()
  action.destroy()
  document.body.innerHTML = ''
})
