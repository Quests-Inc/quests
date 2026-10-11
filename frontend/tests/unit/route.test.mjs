import { beforeEach, expect, test } from 'bun:test'

import { noteIdFromUrl, questIdFromUrl, replaceSearch, searchForView, tabFromUrl } from '../../src/app/model/route.ts'

let replaced = []
function at(search) {
  globalThis.location = { href: `http://x/${search}`, pathname: '/', search }
  globalThis.history = { replaceState: (_s, _t, url) => replaced.push(url) }
}
beforeEach(() => {
  replaced = []
})

test('id из адреса: только положительные числа', () => {
  at('?quest=12&note=0')
  expect(questIdFromUrl()).toBe(12)
  expect(noteIdFromUrl()).toBeNull()
  at('?quest=abc')
  expect(questIdFromUrl()).toBeNull()
  at('')
  expect(questIdFromUrl()).toBeNull()
})

test('tabFromUrl: только известные вкладки', () => {
  at('?tab=graph')
  expect(tabFromUrl()).toBe('graph')
  at('?tab=journal')
  expect(tabFromUrl()).toBeNull()
  at('?tab=evil')
  expect(tabFromUrl()).toBeNull()
})

test('replaceSearch: пишет заново, пустые значения отбрасывает, одинаковый адрес не трогает', () => {
  at('?quest=1')
  replaceSearch({ tab: 'notes', note: null, x: '' })
  expect(replaced).toEqual(['/?tab=notes'])
  replaceSearch({ quest: 1 })
  expect(replaced).toHaveLength(1)
})

test('searchForView: адрес по выбранному объекту вкладки', () => {
  const sel = { quest: 5, note: null, attachment: 9, graph: { type: 'note', id: 3 } }
  expect(searchForView('journal', sel)).toEqual({ quest: 5 })
  expect(searchForView('notes', sel)).toEqual({ tab: 'notes' })
  expect(searchForView('attachments', sel)).toEqual({ attachment: 9 })
  expect(searchForView('graph', sel)).toEqual({ tab: 'graph', gt: 'note', gid: 3 })
  expect(searchForView('history', sel)).toEqual({ tab: 'history' })
  expect(searchForView('journal', { ...sel, quest: null })).toEqual({})
})
