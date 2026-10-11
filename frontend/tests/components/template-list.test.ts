import { afterEach, expect, test } from 'bun:test'

import { mount, unmount } from 'svelte'

import TemplateList from '@/features/templates/ui/TemplateList.svelte'

let cleanup: Array<() => void> = []
afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
})

const tpl = (id: number, extra = {}) => ({ id, title: `Шаблон ${id}`, enabled: true, ...extra })

function render(props: Record<string, unknown>) {
  const calls: string[] = []
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(TemplateList, {
    target,
    props: {
      loading: false,
      templates: [tpl(1), tpl(2, { enabled: false, emit_pool_last_outcome: 'error' })],
      rowMeta: (t: { id: number }) => `сводка ${t.id}`,
      onCreate: () => calls.push('create'),
      onOpen: (t: { id: number }) => calls.push(`open:${t.id}`),
      onMenu: (_e: unknown, t: { id: number }) => calls.push(`menu:${t.id}`),
      onEmit: (t: { id: number }) => calls.push(`emit:${t.id}`),
      onCopy: (t: { id: number }) => calls.push(`copy:${t.id}`),
      onToggleEnabled: (t: { id: number }) => calls.push(`toggle:${t.id}`),
      ...props,
    },
  })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  return { target, calls }
}

test('TemplateList: заглушки загрузки и пустого списка', () => {
  expect(render({ loading: true }).target.textContent).toContain('Загрузка…')
  expect(render({ templates: [] }).target.textContent).toContain('Пока нет шаблонов')
})

test('TemplateList: строки, сводка, пометка падающего пула и выключенный шаблон', () => {
  const { target } = render({})
  const rows = target.querySelectorAll('.tpl-row')
  expect(rows).toHaveLength(2)
  expect(rows[0].textContent).toContain('сводка 1')
  expect(rows[1].classList.contains('tpl-row--off')).toBe(true)
  expect(rows[1].textContent).toContain('пул падает')
})

test('TemplateList: действия строки зовут свои обработчики', () => {
  const { target, calls } = render({})
  const q = (sel: string, i = 0) => target.querySelectorAll(sel)[i] as HTMLElement
  q('.btn--accent').click()
  q('.tpl-row__main').click()
  q('button[aria-label="Эмитировать сейчас"]').click()
  q('button[aria-label="Копировать шаблон"]').click()
  q('.switch').click()
  q('.tpl-row').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true }))
  expect(calls).toEqual(['create', 'open:1', 'emit:1', 'copy:1', 'toggle:1', 'menu:1'])
})
