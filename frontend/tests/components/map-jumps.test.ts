import { expect, test } from 'bun:test'

import { mapJumps } from '@/shared/lib/mapJumps.ts'

test('кнопка карты стоит после ссылки на сущность этого холста и не трогает остальные', async () => {
  const root = document.createElement('div')
  root.innerHTML = '<a href="?note=7">Б</a><a href="?quest=3">Квест</a>'
  const jumps: string[] = []
  const action = mapJumps(root, { keys: 'note:7', jump: (kind, id) => jumps.push(`${kind}:${id}`) })
  const buttons = root.querySelectorAll('button.md-map-jump')
  expect(buttons.length).toBe(1)
  expect(root.querySelector('a')?.nextElementSibling).toBe(buttons[0])
  buttons[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
  expect(jumps).toEqual(['note:7'])
  const late = document.createElement('a')
  late.href = '?note=7'
  late.textContent = 'Позже'
  root.append(late)
  await new Promise((r) => setTimeout(r, 0))
  expect(late.nextElementSibling?.classList.contains('md-map-jump')).toBe(true)
  action.update(null)
  expect(root.querySelector('button')).toBeNull()
  action.destroy()
})
