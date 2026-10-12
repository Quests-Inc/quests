import { beforeEach, expect, test } from 'bun:test'

import { createFoldedGroups } from '@/pages/attachments/model/foldedGroups.svelte.ts'

const KEY = 'quests.attachments.folded'

beforeEach(() => localStorage.removeItem(KEY))

test('toggle сворачивает и разворачивает группу', () => {
  const f = createFoldedGroups()
  expect(f.has('note:1')).toBe(false)
  f.toggle('note:1')
  expect(f.has('note:1')).toBe(true)
  f.toggle('note:1')
  expect(f.has('note:1')).toBe(false)
})

test('свёрнутое запоминается в браузере и читается заново', () => {
  createFoldedGroups().toggle('quest:7')
  expect(JSON.parse(localStorage.getItem(KEY) || '[]')).toEqual(['quest:7'])
  expect(createFoldedGroups().has('quest:7')).toBe(true)
})

test('мусор в хранилище не ломает страницу', () => {
  localStorage.setItem(KEY, '{не json')
  expect(createFoldedGroups().has('x')).toBe(false)
})
