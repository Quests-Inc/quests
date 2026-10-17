import { afterEach, expect, test } from 'bun:test'

import { createCanvas } from '@/pages/scripts/model/canvas.svelte.ts'
import { createScriptEditor } from '@/pages/scripts/model/editor.svelte.ts'

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

const P = (name: string, type: string, extra: object = {}) => ({ name, type, ...extra })
const PORTS: Record<string, any> = {
  'source.step': { inputs: [], outputs: [P('ref', 'ref<step>'), P('title', 'str'), P('progress', 'int')] },
  code: {
    inputs: [P('in', 'value', { optional: true, multi: true })],
    outputs: [P('stdout', 'str'), P('failed', 'doc')],
  },
  'sink.step_progress': {
    inputs: [P('ref', 'ref<step>'), P('progress', 'int', { optional: true, group: 'data' })],
    outputs: [],
  },
}

function serve() {
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const json = (v: unknown) => new Response(JSON.stringify(v), { status: 200 })
    if (key === 'POST /api/script-nodes/ports')
      return json(PORTS[JSON.parse(String(init?.body)).type] ?? { inputs: [], outputs: [] })
    if (key === 'GET /api/script-nodes')
      return json([
        { name: 'source.step', title: 'Источник', kind: 'source', trusted: true },
        { name: 'code', title: 'Код', kind: 'code', trusted: false },
        { name: 'sink.step_progress', title: 'Прогресс', kind: 'sink', trusted: true },
      ])
    if (key === 'POST /api/scripts/validate') return json({ ok: true, problems: [] })
    return new Response('{}', { status: 404 })
  }) as typeof fetch
}

// The stage the gestures report on: a box of a known size.
function stage(w = 1000, h = 600) {
  const el = document.createElement('div')
  Object.defineProperty(el, 'clientWidth', { value: w })
  Object.defineProperty(el, 'clientHeight', { value: h })
  el.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: w, height: h, right: w, bottom: h, x: 0, y: 0 }) as DOMRect
  el.setPointerCapture = () => {}
  return el
}

const script = {
  id: 1,
  title: 's',
  description: '',
  version: 1,
  subjects: ['step'],
  needs_runner: true,
  graph: {
    nodes: [
      { id: 1, type: 'source.step' },
      { id: 2, type: 'code', params: { text: 'true' } },
      { id: 3, type: 'sink.step_progress' },
    ],
    edges: [{ from: { node: 1, port: 'ref' }, to: { node: 3, port: 'ref' } }],
  },
} as any

async function rig() {
  serve()
  const ed = createScriptEditor()
  await ed.open(script)
  const cv = createCanvas(ed)
  cv.el = stage()
  cv.watchSize()
  cv.sync()
  return { ed, cv }
}

test('ноды графа становятся карточками поля: центр и размер, как у карты', async () => {
  const { cv } = await rig()
  expect(cv.views.map((v) => v.id)).toEqual([1, 2, 3])
  expect(cv.views.every((v) => v.w === 220 && v.h > 0)).toBe(true)
  expect(cv.view(2)?.type).toBe('code')
  expect(cv.view(99)).toBeUndefined()
  expect(cv.mini?.nodes).toHaveLength(3) // миникарта карты рисует и их
})

test('вписать: камера показывает все ноды', async () => {
  const { cv } = await rig()
  cv.fit()
  const cam = cv.gest.cam
  expect(cam).not.toEqual({ x: 0, y: 0, k: 1 }) // камера действительно сдвинулась
  expect(cam.k).toBeGreaterThan(0)
  expect(cam.k).toBeLessThanOrEqual(1)
  for (const v of cv.views) {
    const sx = (v.x - v.w / 2) * cam.k + cam.x
    expect(sx).toBeGreaterThanOrEqual(0)
    expect((v.x + v.w / 2) * cam.k + cam.x).toBeLessThanOrEqual(1000)
  }
})

test('перетаскивание карточки возвращается в граф через жесты карты', async () => {
  const { ed, cv } = await rig()
  const v = cv.views[0]
  const before = ed.graph.nodes[0].x
  // жест карты: нажатие на карточку, сдвиг, отпускание
  cv.gest.onCardDown(
    { button: 0, target: cv.el, clientX: 100, clientY: 100, pointerId: 1, stopPropagation() {} } as any,
    v as any
  )
  cv.gest.onMove({ pointerId: 1, clientX: 160, clientY: 130 })
  cv.gest.onUp({ pointerId: 1 })
  await Promise.resolve()
  await Promise.resolve()
  expect(ed.graph.nodes[0].x).not.toBe(before)
  expect(ed.canUndo).toBe(true)
})

test('связь: нажать на выход, отпустить на входе — связь появилась; тип не подходит — отказ', async () => {
  const { ed, cv } = await rig()
  const ev = { clientX: 5, clientY: 5, pointerId: 1, stopPropagation() {}, preventDefault() {} } as any
  cv.startLink({ node: 1, port: 'progress' }, ev)
  expect(cv.pending?.from).toEqual({ node: 1, port: 'progress' })
  await cv.finishLink({ node: 3, port: 'progress' })
  expect(cv.pending).toBeNull()
  expect(ed.graph.edges).toHaveLength(2)

  cv.startLink({ node: 1, port: 'title' }, ev) // str в int-вход не идёт
  await cv.finishLink({ node: 3, port: 'progress' })
  expect(ed.graph.edges).toHaveLength(2)
  expect(ed.error).toContain('не подходит')
})

test('клавиши: Delete убирает выбранное, Ctrl+Z отменяет, Esc сбрасывает незаконченную связь', async () => {
  const { ed, cv } = await rig()
  const key = (k: string, extra: object = {}) =>
    cv.onKey({ key: k, target: document.createElement('div'), preventDefault() {}, ...extra } as any)

  ed.select(2)
  key('Delete')
  await Promise.resolve()
  await Promise.resolve()
  expect(ed.graph.nodes.map((n) => n.id)).toEqual([1, 3])
  key('z', { ctrlKey: true })
  expect(ed.graph.nodes).toHaveLength(3)

  cv.startLink({ node: 1, port: 'ref' }, {
    clientX: 1,
    clientY: 1,
    pointerId: 1,
    stopPropagation() {},
    preventDefault() {},
  } as any)
  key('Escape')
  expect(cv.pending).toBeNull()

  // в поле ввода клавиши не перехватываются
  const input = document.createElement('input')
  cv.onKey({ key: 'Delete', target: input, preventDefault() {} } as any)
  expect(ed.graph.nodes).toHaveLength(3)
})

test('нода из палитры ложится ближе к середине видимого и выбирается', async () => {
  const { ed, cv } = await rig()
  const id = await cv.addAtCenter('code')
  expect(ed.selectedNode).toBe(id)
  expect(ed.graph.nodes).toHaveLength(4)
  cv.sync()
  expect(cv.views).toHaveLength(4)
})

// The field as the page mounts it: its effects must settle (a loop would throw "effect_update_depth_exceeded"), draw every
// node with its ports and a wire per link, and fit the camera once the field has a size.
test('поле смонтировано: ноды и связи нарисованы, эффекты не зацикливаются', async () => {
  const { mount, flushSync, unmount } = await import('svelte')
  const { default: ScriptCanvas } = await import('@/pages/scripts/ui/ScriptCanvas.svelte')
  const { ed, cv } = await rig()
  cv.el = null
  // A field that has a size, as in a browser: that is what lets the camera fit (and what a feedback loop would feed on).
  const proto = HTMLElement.prototype
  const had = Object.getOwnPropertyDescriptor(proto, 'clientWidth')
  Object.defineProperty(proto, 'clientWidth', { configurable: true, get: () => 1000 })
  Object.defineProperty(proto, 'clientHeight', { configurable: true, get: () => 600 })
  const target = document.createElement('div')
  document.body.appendChild(target)
  const app = mount(ScriptCanvas, { target, props: { ed, cv } })
  flushSync()
  for (let i = 0; i < 5; i++) await Promise.resolve()
  flushSync()

  expect(target.querySelectorAll('.snode')).toHaveLength(3)
  expect(target.querySelectorAll('.snode__dot').length).toBeGreaterThan(0)
  expect(target.querySelectorAll('svg.scv__wires path[stroke-width="2"]')).toHaveLength(1) // одна связь графа
  expect(cv.gest.cam).not.toEqual({ x: 0, y: 0, k: 1 }) // и камера вписала ноды
  unmount(app)
  target.remove()
  if (had) Object.defineProperty(proto, 'clientWidth', had)
  else delete (proto as unknown as Record<string, unknown>).clientWidth
})
