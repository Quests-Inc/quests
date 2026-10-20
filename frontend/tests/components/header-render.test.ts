import { afterAll, beforeAll, expect, test } from 'bun:test'

import { createRawSnippet, flushSync, mount, unmount } from 'svelte'

import { sidebars } from '@/shared/lib/sidebar.svelte.ts'
import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
import { JournalHeader } from '@/widgets/journal-header'

// The header's shape: four tabs and two groups, one health meter, the same actions in every section; a list's frame folds and resizes.
beforeAll(() => {
  ;(globalThis as any).ResizeObserver = class {
    observe() {}
    disconnect() {}
    unobserve() {}
  }
})
afterAll(() => {})

const health = { api: 'ok', overlay: 'offline', telegram: 'ok', webdav: 'unknown', db: 'ok' }

function mountHeader(view: string, over: Record<string, unknown> = {}) {
  const target = document.createElement('div')
  document.body.append(target)
  const calls: string[] = []
  const app = mount(JournalHeader, {
    target,
    props: {
      view,
      liveStatus: 'live',
      health,
      onViewChange: (v: string) => calls.push(`view:${v}`),
      onOpenSettings: () => calls.push('settings'),
      onOpenTemplates: () => calls.push('templates'),
      onOpenSecrets: () => calls.push('secrets'),
      onOpenTags: () => calls.push('tags'),
      onOpenTrash: () => calls.push('trash'),
      onOpenCreateQuestline: () => calls.push('questline'),
      onOpenCreateQuest: () => calls.push('quest'),
      onOpenAssistant: () => calls.push('assistant'),
      ...over,
    } as any,
  })
  flushSync()
  return { target, calls, stop: () => (unmount(app), target.remove()) }
}

test('шапка: четыре вкладки и две группы, активная подсвечена, клик переключает', () => {
  const h = mountHeader('notes')
  const tabs = [...h.target.querySelectorAll<HTMLElement>('[role=tab]')]
  expect(tabs).toHaveLength(6)
  expect(tabs.map((t) => t.getAttribute('title'))).toEqual([
    'Журнал',
    'Заметки',
    'Карта',
    'Календарь',
    'Ещё',
    'Автоматика',
  ])
  expect(tabs.filter((t) => t.getAttribute('aria-selected') === 'true').map((t) => t.getAttribute('title'))).toEqual([
    'Заметки',
  ])
  tabs[2].click()
  expect(h.calls).toEqual(['view:graph'])
  h.stop()
})

test('шапка: у открытой вкладки группы кнопка группы носит её имя', () => {
  const h = mountHeader('scripts')
  const group = h.target.querySelector<HTMLElement>('[aria-haspopup=menu][role=tab][aria-selected=true]')
  expect(group?.getAttribute('title')).toBe('Автоматика · Скрипты')
  expect(group?.textContent).toContain('Скрипты')
  h.stop()
})

test('шапка: одинаковые действия справа на любой вкладке, здоровье слева', () => {
  const sets = ['journal', 'graph', 'hooks'].map((v) => {
    const h = mountHeader(v)
    const actions = [...h.target.querySelectorAll('.header-actions button')].map((b) => b.getAttribute('aria-label'))
    const meter = h.target.querySelector('.header-left .hm__btn')
    h.stop()
    return { actions, meter: Boolean(meter) }
  })
  expect(sets[0].actions).toEqual(['Командная строка журнала', 'Новый квест', 'Ещё действия'])
  expect(sets[1]).toEqual(sets[0])
  expect(sets[2]).toEqual(sets[0])
  expect(sets[0].meter).toBe(true)
})

test('здоровье: шесть палочек, офлайн — короткая, не настроено — серое; клик открывает подробности', () => {
  const h = mountHeader('journal')
  const bars = [...h.target.querySelectorAll<HTMLElement>('.hm__btn .hm__bar')].map((b) => b.dataset.level)
  expect(bars).toEqual(['ok', 'ok', 'ok', 'offline', 'ok', 'unknown'])
  const btn = h.target.querySelector<HTMLElement>('.hm__btn')!
  expect(btn.getAttribute('aria-label')).toBe('Сервисы: проблемы — HUD')
  btn.click()
  flushSync()
  expect(h.target.querySelectorAll('.hm__row')).toHaveLength(6)
  h.stop()
})

test('каркас списка: сворачивание хранится по ключу, граница двигается стрелками и сбрасывается Home', () => {
  const target = document.createElement('div')
  document.body.append(target)
  const sidebar = createRawSnippet(() => ({ render: () => '<aside id="s">список</aside>' }))
  const children = createRawSnippet(() => ({ render: () => '<section id="m">работа</section>' }))
  const app = mount(SidebarLayout, { target, props: { key: 'render-test', sidebar, children } as any })
  flushSync()
  const root = target.querySelector<HTMLElement>('.sbl')!
  const handle = target.querySelector<HTMLElement>('.sbl__handle')!
  expect(root.classList.contains('sbl--collapsed')).toBe(false)
  handle.click()
  flushSync()
  expect(root.classList.contains('sbl--collapsed')).toBe(true)
  expect(sidebars.isCollapsed('render-test')).toBe(true)
  expect(handle.getAttribute('aria-label')).toBe('Показать боковую панель')
  handle.click()
  flushSync()
  const sep = target.querySelector<HTMLElement>('[role=separator]')!
  const w0 = sidebars.width
  sep.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
  flushSync()
  expect(sidebars.width).toBe(w0 + 16)
  sep.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true }))
  flushSync()
  expect(root.style.getPropertyValue('--sbl-w')).toBe('264px')
  unmount(app)
  target.remove()
})
