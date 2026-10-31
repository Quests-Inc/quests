import { expect, test } from 'bun:test'

import { duplicateNote } from '@/shared/api/notes.ts'

test('duplicate copies values before returning, preserves privacy, and removes an incomplete copy on failure', async () => {
  const original = globalThis.fetch
  const calls: string[] = []
  let fail = false
  let createdBody: any
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    calls.push(key)
    if (key === 'GET /api/notes/70')
      return Response.json({ id: 70, title: 'Wolf', description: '{{hp}}', is_private: true, parent_id: 5 })
    if (key === 'POST /api/notes') {
      createdBody = JSON.parse(init!.body as string)
      return Response.json({ id: 78 })
    }
    if (key === 'POST /api/notes/78/values/copy') {
      expect(JSON.parse(init!.body as string)).toEqual({ source_id: 70 })
      return fail ? Response.json({ detail: 'copy failed' }, { status: 422 }) : new Response(null, { status: 204 })
    }
    if (key === 'DELETE /api/notes/78') return new Response(null, { status: 204 })
    throw new Error(key)
  }) as typeof fetch
  try {
    expect((await duplicateNote(70)).id).toBe(78)
    expect(createdBody).toMatchObject({ title: 'Wolf (копия)', is_private: true, parent_id: 5, description: '{{hp}}' })
    expect(calls).toEqual(['GET /api/notes/70', 'POST /api/notes', 'POST /api/notes/78/values/copy'])
    calls.length = 0
    fail = true
    await expect(duplicateNote(70)).rejects.toThrow('copy failed')
    expect(calls.at(-1)).toBe('DELETE /api/notes/78')
  } finally {
    globalThis.fetch = original
  }
})
