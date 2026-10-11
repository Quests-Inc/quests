import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import NoteLinks from '@/pages/notes/ui/NoteLinks.svelte'

let cleanup: Array<() => void> = []
afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
})

function render(props: Record<string, unknown>) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(NoteLinks, { target, props })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  return target
}

test('NoteLinks: без ссылок ничего не рисует', () => {
  expect(render({ refs: [], backlinks: [] }).querySelector('.block')).toBeNull()
})

test('NoteLinks: исходящие и входящие, клик отдаёт kind и id, заголовок сворачивает список', () => {
  const opened: string[] = []
  const el = render({
    refs: [{ kind: 'quest', id: 3, title: 'Квест' }],
    backlinks: [{ kind: 'note', id: 8 }],
    onRef: (kind: string, id: number) => opened.push(`${kind}=${id}`),
  })
  expect(el.textContent).toContain('Ссылки (1)')
  expect(el.textContent).toContain('Ссылаются (1)')
  expect(el.textContent).toContain('note=8')
  const links = [...el.querySelectorAll('.notes__link')] as HTMLElement[]
  links[0].click()
  links[1].click()
  expect(opened).toEqual(['quest=3', 'note=8'])
  const toggle = el.querySelector('.block__label--toggle') as HTMLElement
  toggle.click()
  flushSync()
  expect(el.querySelectorAll('.notes__link')).toHaveLength(1)
})
