import { afterEach, expect, test } from 'bun:test'

import { clearStepMirror, getStepMirror, setStepMirror } from '@/shared/api/client.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

test('шаг следит за квестом: чтение, установка с номером квеста в теле, снятие', async () => {
  const calls: { key: string; body?: string }[] = []
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    calls.push({ key, body: init?.body as string | undefined })
    if (key === 'GET /api/steps/5/mirror') return new Response('null', { status: 200 })
    if (key === 'PUT /api/steps/5/mirror') {
      return new Response(JSON.stringify({ step_id: 5, quest_id: 9, parent_quest_id: 2, hook_id: 1, script_id: 3 }), {
        status: 200,
      })
    }
    if (key === 'DELETE /api/steps/5/mirror') return new Response(null, { status: 204 })
    return new Response(JSON.stringify({ detail: 'no route' }), { status: 404 })
  }) as typeof fetch

  expect(await getStepMirror(5)).toBeNull()
  const m = await setStepMirror(5, 9)
  expect(m.quest_id).toBe(9)
  await clearStepMirror(5)
  expect(calls.map((c) => c.key)).toEqual([
    'GET /api/steps/5/mirror',
    'PUT /api/steps/5/mirror',
    'DELETE /api/steps/5/mirror',
  ])
  expect(JSON.parse(calls[1].body!)).toEqual({ quest_id: 9 })
})
