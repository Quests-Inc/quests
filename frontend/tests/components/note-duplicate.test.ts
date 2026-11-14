import { expect, test } from 'bun:test'

import { duplicateNote } from '@/shared/api/notes.ts'

test('duplicate uses one atomic server operation and reports failures without creating partial notes', async () => {
  const original = globalThis.fetch
  const calls: string[] = []
  let fail = false
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    calls.push(`${init?.method ?? 'GET'} ${url}`)
    expect(JSON.parse(init!.body as string)).toEqual({})
    return fail ? Response.json({ detail: 'copy failed' }, { status: 422 }) : Response.json({ id: 78 })
  }) as typeof fetch
  try {
    expect((await duplicateNote(70)).id).toBe(78)
    expect(calls).toEqual(['POST /api/notes/70/duplicate'])
    fail = true
    await expect(duplicateNote(70)).rejects.toThrow('copy failed')
    expect(calls).toHaveLength(2)
  } finally {
    globalThis.fetch = original
  }
})
