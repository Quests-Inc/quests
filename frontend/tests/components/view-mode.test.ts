import { afterEach, expect, test } from 'bun:test'

import { flushSync } from 'svelte'

import { createViewMode, loadViewMode } from '@/pages/notes/model/viewMode.svelte.ts'

import { inRoot } from './root.svelte.ts'

const store = (initial: string | null) => {
  const data: Record<string, string> = initial ? { 'quests.notes.viewMode': initial } : {}
  return { data, getItem: (k: string) => data[k] ?? null, setItem: (k: string, v: string) => (data[k] = v) }
}

function narrow(matches: boolean) {
  const listeners: Array<() => void> = []
  const mq = { matches, addEventListener: (_: string, fn: () => void) => listeners.push(fn), removeEventListener() {} }
  ;(window as any).matchMedia = () => mq
  return { change: (m: boolean) => ((mq.matches = m), listeners.forEach((fn) => fn())) }
}

afterEach(() => {
  delete (window as any).matchMedia
})

test('loadViewMode: сохранённое читается, мусор — «оба»', () => {
  expect(loadViewMode(store('raw'))).toBe('raw')
  expect(loadViewMode(store('bogus'))).toBe('combined')
  expect(loadViewMode(null)).toBe('combined')
})

test('режим запоминается; на узком экране «Оба» превращается в «Просмотр»', () => {
  const media = narrow(false)
  const s = store(null)
  let vm!: ReturnType<typeof createViewMode>
  const cleanup = inRoot(() => {
    vm = createViewMode(s)
  })
  flushSync()
  expect(vm.mode).toBe('combined')
  vm.set('raw')
  expect(s.data['quests.notes.viewMode']).toBe('raw')
  vm.set('combined')
  media.change(true)
  flushSync()
  expect(vm.mode).toBe('formatted')
  cleanup()
})
