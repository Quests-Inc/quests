import { expect, test } from 'bun:test'

import {
  clearGraphPresence,
  focusGraphPresence,
  graphPresence,
  setGraphPresence,
} from '@/shared/lib/graphPresence.svelte.ts'

test('setGraphPresence publishes keys and focus handler', () => {
  clearGraphPresence()
  expect(graphPresence.keys).toEqual([])
  let hit = ''
  setGraphPresence(['attachment:7', 'quest:1'], (key) => {
    hit = key
  })
  expect(graphPresence.keys).toEqual(['attachment:7', 'quest:1'])
  focusGraphPresence('attachment:7')
  expect(hit).toBe('attachment:7')
  clearGraphPresence()
  expect(graphPresence.keys).toEqual([])
  focusGraphPresence('attachment:7')
  expect(hit).toBe('attachment:7')
})
