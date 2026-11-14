import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

mock.module('@/shared/api/http.ts', () => ({
  BASE: '',
  onUnauthorized: null,
  setUnauthorizedHandler() {},
  UnauthorizedError: class extends Error {},
  quietQs: () => '',
  request: async () => ({ effective: [] }),
}))

let requests = 0
let fail = false
const ending = 'Конец полной заметки'
mock.module('@/shared/api/notes.ts', () => ({
  getNote: async () => {
    requests++
    if (fail) throw new Error('offline')
    return { description: `${'Текст '.repeat(900)}\n\n${ending}` }
  },
}))
const { default: Card } = await import('@/widgets/graph-canvas/ui/GraphCard.svelte')

async function show(extra: Record<string, unknown> = {}) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(Card, {
    target,
    props: {
      n: {
        key: 'note:12',
        id: 12,
        type: 'note',
        title: 'Note',
        desc: 'a'.repeat(4000) + '…',
        x: 0,
        y: 0,
        w: 280,
        h: 300,
        links: 0,
        depth: 0,
      },
      ready: true,
      onCardDown() {},
      onCtx() {},
      onStartEdit() {},
      onFinishEdit() {},
      onOpenMore() {},
      onShowChanges() {},
      ...extra,
    },
  })
  flushSync()
  await new Promise((resolve) => setTimeout(resolve, 0))
  flushSync()
  return {
    target,
    async close() {
      await unmount(app)
      target.remove()
    },
  }
}

test('a readable note loads and displays text beyond the graph preview', async () => {
  requests = 0
  const view = await show()
  expect(requests).toBe(1)
  expect(view.target.textContent).toContain(ending)
  await view.close()
})

test('overview cards and draft edits do not load note bodies', async () => {
  requests = 0
  for (const extra of [{ tier: 'lite' }, { tier: 'title' }, { ready: false }, { changed: true }]) {
    const view = await show(extra)
    await view.close()
  }
  expect(requests).toBe(0)
})

test('failed full text loading can be retried', async () => {
  fail = true
  const view = await show()
  fail = false
  const retry = [...view.target.querySelectorAll('button')].find((button) => button.textContent?.includes('Повторить'))!
  expect(retry).toBeDefined()
  retry.click()
  flushSync()
  await new Promise((resolve) => setTimeout(resolve, 0))
  flushSync()
  expect(view.target.textContent).toContain(ending)
  await view.close()
})
