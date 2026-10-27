import { afterEach, expect, test } from 'bun:test'

import { emptyTrash, listTrash, purgeFromTrash, restoreFromTrash } from '@/shared/api/client.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

function fakeFetch(routes: Record<string, unknown>) {
  const calls: string[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    calls.push(key)
    if (!(key in routes)) return new Response(JSON.stringify({ detail: `no route ${key}` }), { status: 404 })
    const body = routes[key]
    return body === null ? new Response(null, { status: 204 }) : new Response(JSON.stringify(body), { status: 200 })
  }) as typeof fetch
  return calls
}

test('listTrash: квесты, заметки, скрипты и хуки одним списком, свежеудалённое первым', async () => {
  fakeFetch({
    'GET /api/trash/quests': [{ kind: 'quest', id: 1, title: 'q', deleted_at: '2026-10-17 10:00:00' }],
    'GET /api/trash/notes': [
      { kind: 'note', id: 7, title: 'n', deleted_at: '2026-10-17 12:00:00' },
      { kind: 'note', id: 8, title: 'old', deleted_at: '2026-10-16 09:00:00' },
    ],
    'GET /api/trash/scripts': [{ kind: 'script', id: 4, title: 's', deleted_at: '2026-10-17 11:00:00' }],
    'GET /api/trash/hooks': [],
  })
  const items = await listTrash()
  expect(items.map((i) => `${i.kind}:${i.id}`)).toEqual(['note:7', 'script:4', 'quest:1', 'note:8'])
})

test('restore, purge и очистка идут в нужные маршруты по виду сущности', async () => {
  const calls = fakeFetch({
    'POST /api/trash/quests/3/restore': null,
    'DELETE /api/trash/notes/9': null,
    'DELETE /api/trash/quests': null,
    'DELETE /api/trash/notes': null,
    'DELETE /api/trash/hooks': null,
    'DELETE /api/trash/scripts': null,
  })
  await restoreFromTrash('quest', 3)
  await purgeFromTrash('note', 9)
  await emptyTrash()
  expect(calls).toEqual([
    'POST /api/trash/quests/3/restore',
    'DELETE /api/trash/notes/9',
    'DELETE /api/trash/quests',
    'DELETE /api/trash/notes',
    'DELETE /api/trash/hooks',
    'DELETE /api/trash/scripts',
  ])
})
