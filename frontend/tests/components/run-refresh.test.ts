import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'
const real = await import('@/shared/api/client.ts')
let resolveRefresh: (value: unknown) => void = () => {}
let rejectRefresh: (reason: Error) => void = () => {}
let reads = 0
const detail = {
  id: 144,
  state: 'waiting',
  graph_ref: 'script:13:2',
  source_kind: 'template',
  source_id: 5,
  subject_type: 'template',
  subject_id: 5,
  created_at: '2026-10-19T16:30:00Z',
  nodes: [],
  jobs: [],
}
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listRuns: async () => [{ ...detail, script_title: 'Почта', source_title: 'Почта' }],
  getScriptNodes: async () => [],
  getRun: async () =>
    ++reads === 1
      ? detail
      : new Promise((resolve, reject) => {
          resolveRefresh = resolve
          rejectRefresh = reject
        }),
}))
const { default: RunsPage } = await import('@/pages/runs/ui/RunsPage.svelte')
const settle = async () => {
  await Promise.resolve()
  await Promise.resolve()
  flushSync()
}
test('poll keeps the detail visible and applies the new state without a loading placeholder', async () => {
  history.replaceState({}, '', '/?tab=runs&run=144&src=template:5')
  let poll: (() => void) | undefined
  let retry: (() => void) | undefined
  const original = globalThis.setTimeout
  globalThis.setTimeout = ((fn: () => void, ms?: number, ...args: unknown[]) => {
    if (ms === 1500) {
      poll = fn
      return 987654
    }
    if (ms === 5000) {
      retry = fn
      return 987655
    }
    return original(fn, ms, ...args)
  }) as typeof setTimeout
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RunsPage, { target })
  try {
    flushSync()
    await settle()
    await settle()
    expect(target.textContent).toContain('Почта')
    expect(poll).toBeDefined()
    poll!()
    await settle()
    expect(reads).toBe(2)
    expect(target.textContent).not.toContain('Загрузка запуска…')
    expect(target.querySelector('.rd__head')).not.toBeNull()
    rejectRefresh(new Error('offline'))
    await settle()
    await settle()
    expect(target.textContent).toContain('Не удалось обновить запуск: offline')
    expect(target.querySelector('.rd__head')).not.toBeNull()
    expect(retry).toBeDefined()
    retry!()
    await settle()
    resolveRefresh({ ...detail, state: 'done' })
    await settle()
    await settle()
    expect(target.querySelector('.rd__state')?.textContent).toBe('готово')
  } finally {
    await unmount(app)
    target.remove()
    globalThis.setTimeout = original
  }
})
