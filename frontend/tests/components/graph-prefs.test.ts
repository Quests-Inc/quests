import { expect, test } from 'bun:test'

import { createGraphPrefs } from '@/pages/graph/model/prefs.svelte.ts'

function fakeStore(initial: Record<string, string> = {}, broken = false) {
  const data = { ...initial }
  return {
    data,
    getItem: (k: string) => {
      if (broken) throw new Error('no storage')
      return data[k] ?? null
    },
    setItem: (k: string, v: string) => {
      if (broken) throw new Error('no storage')
      data[k] = v
    },
  }
}

test('по умолчанию: раскладка «дерево», только активные квесты, все виды, ничего не скрыто', () => {
  const p = createGraphPrefs(fakeStore())
  expect(p.layoutMode).toBe('tree')
  expect([...p.questStatuses]).toEqual(['active'])
  expect(p.statusLabel).toBe('активен')
  expect(p.shownTypes.size).toBe(5)
  expect(p.typesLabel).toBe('все')
  expect(p.hidden.size).toBe(0)
  expect(p.expandKeys).toEqual([])
})

test('сохранённое читается, мусор отбрасывается', () => {
  const p = createGraphPrefs(
    fakeStore({
      'quests.graph.layout': 'radial',
      'quests.graph.hidden': '["note:1", 5, "quest:2"]',
      'quests.graph.statuses': '["completed", "bogus"]',
      'quests.graph.types': '["quest", "nope"]',
      'quests.graph.expand': '["quest:5>note"]',
    })
  )
  expect(p.layoutMode).toBe('radial')
  expect([...p.hidden]).toEqual(['note:1', 'quest:2'])
  expect([...p.questStatuses]).toEqual(['completed'])
  expect([...p.shownTypes]).toEqual(['quest'])
  expect(p.typesLabel).toBe('1 из 5')
  expect(p.expandKeys).toEqual(['quest:5>note'])
  expect(createGraphPrefs(fakeStore({ 'quests.graph.layout': 'wat' })).layoutMode).toBe('tree')
})

test('переключатели пишут в хранилище; последний статус и вид не выключить', () => {
  const store = fakeStore()
  const p = createGraphPrefs(store)
  p.toggleStatus('failed')
  expect(p.statusLabel).toBe('2 статуса')
  expect(JSON.parse(store.data['quests.graph.statuses'])).toEqual(['active', 'failed'])
  p.toggleStatus('failed')
  p.toggleStatus('active')
  expect([...p.questStatuses]).toEqual(['active'])
  p.toggleType('note')
  expect(p.shownTypes.has('note')).toBe(false)
  for (const t of [...p.shownTypes]) p.toggleType(t)
  expect(p.shownTypes.size).toBe(1)
})

test('скрытые, раскрытые связи и раскладка', () => {
  const store = fakeStore()
  const p = createGraphPrefs(store)
  p.saveHidden(new Set(['quest:9']))
  expect(JSON.parse(store.data['quests.graph.hidden'])).toEqual(['quest:9'])
  p.setExpand(['quest:5', 'quest:5>note', 'note:1'])
  expect(p.ownExpand('quest:5')).toEqual(['quest:5', 'quest:5>note'])
  p.layoutMode = 'layers'
  expect(store.data['quests.graph.layout']).toBe('layers')
})

test('без хранилища или с ломаным — работает до перезагрузки', () => {
  for (const store of [null, fakeStore({}, true)]) {
    const p = createGraphPrefs(store)
    p.toggleStatus('failed')
    p.layoutMode = 'down'
    p.saveHidden(new Set(['a:1']))
    expect(p.statusLabel).toBe('2 статуса')
    expect(p.hidden.size).toBe(1)
  }
})

test('настройки деревьев переживают перезагрузку и восстанавливаются независимо', () => {
  const values = new Map<string, string>()
  const store = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
  const p = createGraphPrefs(store)
  p.setTree('quest:5', { depth: 1, layout: 'radial' })
  p.setTree('note:8', { depth: 3, layout: 'down' })
  const restored = createGraphPrefs(store)
  expect(restored.trees).toEqual({ 'quest:5': { depth: 1, layout: 'radial' }, 'note:8': { depth: 3, layout: 'down' } })
  restored.restoreTrees({ 'quest:5': { depth: 2, layout: 'tree' }, bad: { depth: 20, layout: 'missing' } })
  expect(restored.trees).toEqual({ 'quest:5': { depth: 2, layout: 'tree' } })
})

test('зоны сохраняются в браузере и проверяются при восстановлении', () => {
  const values = new Map<string, string>()
  const store = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
  const p = createGraphPrefs(store)
  const z = { id: 'a', title: 'Работа', color: '#6a8fbf', x: 10, y: -20, w: 600, h: 400 }
  p.restoreZones([z, { ...z, id: 'invalid', w: 0 }])
  expect(createGraphPrefs(store).zones).toEqual([z])
  p.restoreZones([])
  expect(createGraphPrefs(store).zones).toEqual([])
})
