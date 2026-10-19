import { afterAll, beforeAll, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { createCanvas } from '@/pages/scripts/model/canvas.svelte.ts'
import { createScriptEditor } from '@/pages/scripts/model/editor.svelte.ts'

// The field must draw the nodes of the open script: a card per node. (A refactor once dropped the effect that builds the views,
// and every script opened as an empty field.)
const realFetch = globalThis.fetch
beforeAll(() => {
  ;(globalThis as any).ResizeObserver = class {
    observe() {}
    disconnect() {}
    unobserve() {}
  }
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200 })
    if (key === 'GET /api/script-nodes') return json([{ name: 'code', title: 'Код', kind: 'code', trusted: false }])
    if (key === 'POST /api/script-nodes/ports') return json({ inputs: [], outputs: [{ name: 'stdout', type: 'str' }] })
    if (key === 'POST /api/scripts/validate') return json({ ok: true, problems: [] })
    return new Response('{}', { status: 404 })
  }) as typeof fetch
})
afterAll(() => {
  globalThis.fetch = realFetch
})

test('a script opens with a card for each of its nodes', async () => {
  const ScriptCanvas = (await import('@/pages/scripts/ui/ScriptCanvas.svelte')).default
  const ed = createScriptEditor()
  const cv = createCanvas(ed)
  await ed.open({
    id: 1,
    title: 'с кодом',
    description: '',
    purpose: '',
    version: 1,
    subjects: [],
    needs_runner: true,
    graph: {
      nodes: [
        { id: 1, type: 'code', params: { text: 'echo 1' } },
        { id: 2, type: 'code', params: { text: 'echo 2' } },
      ],
      edges: [],
    },
  } as any)
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(ScriptCanvas, { target, props: { ed, cv } })
  flushSync()
  await new Promise((r) => setTimeout(r, 20))
  flushSync()
  expect(target.querySelectorAll('[data-gkey]')).toHaveLength(2)
  unmount(app)
  target.remove()
})
