import { afterEach, expect, test } from 'bun:test'

import {
  browseRepository,
  listRepositories,
  resetRepositorySession,
  updateRepository,
} from '@/shared/api/repositories.ts'
import { repositoryRefs } from '@/shared/lib/repositoryRefs.svelte.ts'
const originalFetch = globalThis.fetch
afterEach(() => {
  resetRepositorySession()
  globalThis.fetch = originalFetch
})

test('session cache coalesces visits, isolates branches/commits, refreshes and invalidates edits', async () => {
  const calls: string[] = []
  globalThis.fetch = (async (input) => {
    calls.push(String(input))
    return new Response(JSON.stringify({ tree: [], commits: [], branches: [] }))
  }) as typeof fetch
  const controller = new AbortController()
  const first = browseRepository(1, 'main', '', controller.signal)
  controller.abort()
  const second = browseRepository(1, 'main', '')
  await expect(first).rejects.toThrow('Aborted')
  await second
  await browseRepository(1, 'main', '')
  expect(calls).toHaveLength(1)
  await browseRepository(1, 'other', '')
  await browseRepository(1, 'main', '', undefined, 'abc')
  expect(calls).toHaveLength(3)
  await browseRepository(1, 'main', '', undefined, '', true)
  expect(calls.at(-1)).toContain('refresh=1')
  await browseRepository(1, 'main', '')
  expect(calls).toHaveLength(4)
  await updateRepository(1, { token: 'test-only' })
  await browseRepository(1, 'main', '')
  expect(calls).toHaveLength(6)
  resetRepositorySession()
  await browseRepository(1, 'main', '')
  expect(calls).toHaveLength(7)
})

test('failures can be retried; catalog supplies labels and clears on logout', async () => {
  let calls = 0
  globalThis.fetch = (async () => {
    calls++
    if (calls === 1) throw new Error('offline')
    return new Response(JSON.stringify({ items: [{ id: 8, title: 'Quests' }], quota_bytes: 1 }))
  }) as typeof fetch
  await expect(listRepositories()).rejects.toThrow('offline')
  await Promise.all([listRepositories(), listRepositories()])
  expect(calls).toBe(2)
  expect(repositoryRefs.labels['repo:8']).toBe('Quests')
  resetRepositorySession()
  expect(repositoryRefs.labels).toEqual({})
})
