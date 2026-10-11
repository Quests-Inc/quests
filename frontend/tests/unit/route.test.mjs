import { beforeEach, expect, test } from 'bun:test'

import {
  healthFrom,
  healthOffline,
  initialRoute,
  noteIdFromUrl,
  questIdFromUrl,
  replaceSearch,
  searchForView,
  tabFromUrl,
} from '../../src/app/model/route.ts'

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

test('initialRoute: вложение важнее заметки, заметка — вкладки, вкладка — квеста', () => {
  at('?attachment=4&note=2&tab=graph&quest=9')
  expect(initialRoute()).toEqual({ view: 'attachments', attachmentId: 4 })
  at('?note=2&tab=graph&quest=9')
  expect(initialRoute()).toEqual({ view: 'notes', noteId: 2 })
  at('?tab=history&quest=9')
  expect(initialRoute()).toEqual({ view: 'history' })
  at('?quest=9')
  expect(initialRoute()).toEqual({ questId: 9 })
  at('')
  expect(initialRoute()).toEqual({})
})

test('initialRoute: центр карты берётся из gt и gid только если они корректны', () => {
  at('?tab=graph&gt=note&gid=7')
  expect(initialRoute()).toEqual({ view: 'graph', graphRoot: { type: 'note', id: 7 } })
  at('?tab=graph&gt=evil&gid=7')
  expect(initialRoute()).toEqual({ view: 'graph' })
  at('?tab=graph&gt=quest&gid=0')
  expect(initialRoute()).toEqual({ view: 'graph' })
})

test('healthFrom и healthOffline: чипы шапки', () => {
  const h = healthFrom({
    api: { status: 'ok' },
    components: {
      overlay: { status: 'ok' },
      telegram: { status: 'offline' },
      webdav: { status: 'weird' },
      db: { status: 'warn' },
    },
  })
  expect(h).toMatchObject({ api: 'ok', overlay: 'ok', telegram: 'offline', webdav: 'unknown', db: 'warn' })
  expect(healthFrom(null)).toMatchObject({ api: 'offline', overlay: 'offline', webdav: 'unknown', db: 'unknown' })
  expect(healthOffline()).toEqual({
    api: 'offline',
    overlay: 'unknown',
    telegram: 'unknown',
    webdav: 'unknown',
    db: 'unknown',
  })
})
