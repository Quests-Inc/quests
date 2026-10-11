import { beforeEach, expect, test } from 'bun:test'

import { createNavigation, TAB_ORDER } from '@/app/model/navigation.svelte.ts'

const quests = [
  { id: 1, questline_id: 7, steps: [{ id: 10 }] },
  { id: 2, questline_id: null, steps: [] },
]

function make() {
  const calls: string[] = []
  const nav = createNavigation({
    quests: () => quests,
    revealDetail: (t) => calls.push(`detail:${t}`),
    revealList: (t) => calls.push(`list:${t}`),
    openTemplate: (id) => calls.push(`tpl:${id}`),
  })
  return { nav, calls }
}
const url = () => location.pathname + location.search

beforeEach(() => history.replaceState(null, '', '/'))

test('start читает вкладку и выбранное из адреса', () => {
  history.replaceState(null, '', '/?note=4&tab=graph')
  const { nav } = make()
  nav.start()
  expect(nav.view).toBe('notes')
  expect(nav.selectedNoteId).toBe(4)
  history.replaceState(null, '', '/?quest=2')
  const b = make().nav
  b.start()
  expect(b.selectedId).toBe(2)
  expect(b.pendingSelectId).toBe(2)
})

test('setView меняет вкладку, направление слайда и адрес', () => {
  const { nav } = make()
  nav.setView('graph')
  expect(nav.view).toBe('graph')
  expect(nav.navDir).toBe(1)
  expect(url()).toBe('/?tab=graph')
  nav.setView('notes')
  expect(nav.navDir).toBe(-1)
  expect(url()).toBe('/?tab=notes')
})

test('swipeTab листает соседей и не выходит за край', () => {
  const { nav } = make()
  nav.swipeTab(-1)
  expect(nav.view).toBe('journal')
  nav.swipeTab(1)
  expect(nav.view).toBe(TAB_ORDER[1])
})

test('selectQuest: невалидный id игнорируется, валидный открывает журнал и пишет адрес', () => {
  const { nav, calls } = make()
  nav.setView('notes')
  nav.selectQuest(0)
  nav.selectQuest(NaN)
  expect(nav.selectedId).toBeNull()
  nav.selectQuest(2)
  expect(nav.view).toBe('journal')
  expect(nav.selectedId).toBe(2)
  expect(nav.pendingSelectId).toBe(2)
  expect(url()).toBe('/?quest=2')
  expect(calls).toEqual(['detail:journal'])
  nav.selectQuest(1, { pushUrl: false })
  expect(url()).toBe('/?quest=2')
})

test('selectNote / selectAttachment: выбор переключает вкладку, сброс возвращает к списку', () => {
  const { nav, calls } = make()
  nav.selectNote(5)
  expect(nav.view).toBe('notes')
  expect(url()).toBe('/?note=5')
  nav.selectNote(null)
  expect(nav.selectedNoteId).toBeNull()
  expect(calls).toEqual(['detail:notes', 'list:notes'])
  nav.selectAttachment(9)
  expect(nav.view).toBe('attachments')
  expect(url()).toBe('/?attachment=9')
  nav.selectAttachment(null)
  expect(nav.selectedAttachmentId).toBeNull()
})

test('clearQuest снимает выбор и ожидание, показывает список', () => {
  const { nav, calls } = make()
  nav.selectQuest(1)
  nav.clearQuest()
  expect(nav.selectedId).toBeNull()
  expect(nav.pendingSelectId).toBeNull()
  expect(calls.at(-1)).toBe('list:journal')
  expect(url()).toBe('/')
})

test('openGraph ставит центр и вкладку; onGraphCenter(null) очищает центр', () => {
  const { nav } = make()
  nav.openGraph('note', '3')
  expect(nav.view).toBe('graph')
  expect(nav.graphRoot).toEqual({ type: 'note', id: 3 })
  expect(url()).toBe('/?tab=graph&gt=note&gid=3')
  nav.onGraphCenter(null, null)
  expect(nav.graphRoot).toEqual({ type: null, id: null })
})

test('openHistoryDay пишет день в адрес до смены вкладки', () => {
  const { nav } = make()
  nav.openHistoryDay('2026-03-04')
  expect(nav.view).toBe('history')
  expect(url()).toBe('/?tab=history&day=2026-03-04')
})

test('openRef: разбор ссылок по видам', () => {
  const { nav, calls } = make()
  nav.openRef('questline', 7)
  expect(nav.selectedId).toBe(1)
  nav.openRef('questline', 99)
  expect(nav.view).toBe('journal')
  nav.openRef('step', 10)
  expect(nav.selectedId).toBe(1)
  nav.openRef('template', 4)
  expect(calls).toContain('tpl:4')
  nav.openRef('note', 8)
  expect(nav.view).toBe('notes')
  nav.openRef('attachment', 3)
  expect(nav.view).toBe('attachments')
  nav.openRef('unknown', 1)
})
