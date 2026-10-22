import { afterEach, expect, test } from 'bun:test'

import { bakeAutoScripts } from '@/shared/lib/scriptBake.ts'

let restore: (() => void) | null = null
afterEach(() => {
  restore?.()
  restore = null
})

function mockEval(value: (arg: string) => string, fail = false) {
  const calls: string[] = []
  const realFetch = globalThis.fetch
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push(`${url} ${init?.body}`)
    const arg = JSON.parse(String(init?.body)).args._ as string
    if (fail)
      return new Response(JSON.stringify({ detail: 'нет' }), { status: 422, headers: { 'Content-Type': 'application/json' } })
    return new Response(JSON.stringify({ value: value(arg), version: 1 }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
  restore = () => {
    globalThis.fetch = realFetch
  }
  return calls
}

test('?auto becomes the value, a click embedding and code stay as written', async () => {
  const calls = mockEval((arg) => (arg === '8+pb+str' ? '11' : '4'))
  const out = await bakeAutoScripts(
    'КД script=1{8+pb+str}?auto, удар script=1{1к20+dex}, код `script=1{8+pb+str}?auto`.',
    { kind: 'note', id: 3 }
  )
  expect(out).toBe('КД 8+pb+str = 11, удар script=1{1к20+dex}, код `script=1{8+pb+str}?auto`.')
  expect(calls).toEqual(['/api/scripts/1/eval {"args":{"_":"8+pb+str"},"host":{"kind":"note","id":3}}'])
})

test('?hide drops the formula: a bare ? until auto fills in only the value', async () => {
  const calls = mockEval(() => '4')
  const out = await bakeAutoScripts('бонус script=1{dex+pb}?auto&hide, бросок script=1{1к20+dex+pb}?hide.')
  expect(out).toBe('бонус 4, бросок ?.')
  expect(calls).toHaveLength(1)
})

test('a hidden formula that fails stays a question mark', async () => {
  mockEval(() => '', true)
  expect(await bakeAutoScripts('script=1{2*con}?auto&hide')).toBe('?')
})

test('a star in the formula is escaped', async () => {
  mockEval(() => '9')
  expect(await bakeAutoScripts('script=1{2*con}?auto')).toBe('2\\*con = 9')
})

test('a failed roll stays in the text', async () => {
  mockEval(() => '', true)
  expect(await bakeAutoScripts('script=1{2*con}?auto')).toBe('2\\*con — ошибка')
})

test('the seventeenth ?auto is left for a click', async () => {
  const calls = mockEval(() => '1')
  const src = Array.from({ length: 17 }, () => 'script=1{1}?auto').join(' ')
  const out = await bakeAutoScripts(src)
  expect(calls).toHaveLength(16)
  expect(out.endsWith('script=1{1}?auto')).toBe(true)
  expect(out.slice(0, out.lastIndexOf('script=')).trim().endsWith('= 1')).toBe(true)
})
