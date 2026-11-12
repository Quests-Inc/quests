import { afterEach, expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { searchFilters, searchTypes } from '@/shared/lib/entitySearch.svelte.ts'
const calls: { q: string; offset?: number; signal?: AbortSignal }[] = []
let finishOld: (v: unknown) => void = () => {}
mock.module('@/shared/api/entitySearch.ts', () => ({
  searchEntities: async (q: { q: string; offset?: number }, signal?: AbortSignal) => {
    calls.push({ ...q, signal })
    if (q.q === 'old')
      return new Promise((resolve) => {
        finishOld = resolve
      })
    return q.offset
      ? { hits: [{ kind: 'note', id: 2, title: 'Вторая' }], more: false, next: 2 }
      : { hits: [{ kind: 'note', id: 1, title: 'Первая' }], more: true, next: 1 }
  },
}))
const { default: Harness } = await import('./EntitySearchHarness.svelte')
let app: ReturnType<typeof mount> | undefined
afterEach(() => {
  if (app) unmount(app)
  app = undefined
  document.body.innerHTML = ''
  calls.length = 0
})
const settle = () => new Promise((r) => setTimeout(r, 20))
function type(q: string) {
  const input = document.querySelector('input')!
  input.value = q
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}
test('server pages are combined and late replies cannot replace a new query', async () => {
  app = mount(Harness, { target: document.body })
  flushSync()
  type('old')
  await settle()
  type('new')
  await settle()
  flushSync()
  expect(document.querySelector('output')?.textContent).toBe('1,2')
  expect(calls.find((c) => c.q === 'old')?.signal?.aborted).toBe(true)
  finishOld({ hits: [{ kind: 'note', id: 999, title: 'Устарело' }], more: false, next: 1 })
  await settle()
  flushSync()
  expect(document.querySelector('output')?.textContent).toBe('1,2')
  type('')
  await settle()
  flushSync()
  expect(document.querySelector('output')?.textContent).toBe('пусто')
})
test('parsed flags preserve alternatives, conjunctions and negative tokens', () => {
  const result = searchFilters(
    {
      text: '',
      terms: [
        { key: 'status', value: 'active', neg: false },
        { key: 'status', value: 'done', neg: false },
        { key: 'flag', value: 'pinned', neg: false },
        { key: 'flag', value: 'hidden', neg: true },
      ],
      unknown: [],
      active: true,
    },
    { status: { label: '', test: () => true }, flag: { label: '', all: true, test: () => true } }
  )
  expect(result).toEqual([
    { key: 'status', values: ['active', 'done'], all: false },
    { key: 'flag', values: ['pinned'], all: true },
    { key: 'flag', values: ['hidden'], not: true },
  ])
})

test('mixed kind selection unions positive types before applying exclusions', () => {
  const parsed = {
    text: '',
    terms: [
      { key: 'type', value: 'quest', neg: false },
      { key: 'type', value: 'note', neg: false },
      { key: 'type', value: 'quest', neg: true },
    ],
    unknown: [],
    active: true,
  }
  expect(searchTypes(parsed, ['quest', 'note', 'questline'])).toEqual(['note'])
})
