import { expect, test } from 'bun:test'

import { createGraphGestures } from '@/widgets/graph-canvas/model/gestures.svelte.ts'

function stage() {
  const el = document.createElement('div')
  el.getBoundingClientRect = () =>
    ({ left: 10, top: 20, width: 800, height: 600, right: 810, bottom: 620, x: 10, y: 20 }) as DOMRect
  ;(el as any).setPointerCapture = () => {}
  document.body.append(el)
  return el
}

const node = (key: string, x: number, y: number) => ({
  key,
  type: 'quest',
  id: 1,
  title: key,
  x,
  y,
  w: 100,
  h: 60,
  links: 0,
  depth: 0,
})

function make(extra: Record<string, unknown> = {}) {
  const el = stage()
  const nodes = [node('quest:1', 0, 0), node('quest:2', 400, 0)]
  const state = {
    picked: new Set<string>(),
    saved: new Map<string, any>(),
    selected: [] as string[],
    touched: 0,
    linkCursor: null as any,
    selectMode: false,
    linking: false,
    finished: [] as string[],
  }
  const g = createGraphGestures({
    getStage: () => el,
    nodes: () => nodes as any,
    nodeMap: () => new Map(nodes.map((n) => [n.key, n])) as any,
    touchView: () => state.touched++,
    isLinking: () => state.linking,
    finishLink: (n) => state.finished.push(n.key),
    setLinkCursor: (p) => (state.linkCursor = p),
    selectMode: () => state.selectMode,
    getPicked: () => state.picked,
    setPicked: (s) => (state.picked = s),
    getSaved: () => state.saved,
    select: (n) => state.selected.push(n.key),
    ...extra,
  })
  const target = document.createElement('div')
  el.append(target)
  const ev = (x: number, y: number, extra: Record<string, unknown> = {}) =>
    ({
      clientX: x,
      clientY: y,
      button: 0,
      pointerId: 1,
      pointerType: 'mouse',
      target,
      preventDefault() {},
      stopPropagation() {},
      ...extra,
    }) as any
  return { g, nodes, state, ev, el }
}

test('панорама: перетаскивание фона сдвигает камеру', () => {
  const { g, ev } = make()
  g.onBgDown(ev(100, 100))
  g.onMove(ev(130, 90))
  g.onUp(ev(130, 90))
  expect(g.cam).toMatchObject({ x: 30, y: -10, k: 1 })
})

test('колесо: зум к точке под курсором, граница масштаба', () => {
  const { g } = make()
  g.zoomAt(2, 410, 320)
  expect(g.cam.k).toBe(2)
  expect(g.cam.x).toBe(-410)
  g.zoomAt(100, 0, 0)
  expect(g.cam.k).toBe(2.5)
  g.onWheel({
    target: document.createElement('div'),
    deltaY: -100,
    ctrlKey: false,
    clientX: 10,
    clientY: 20,
    preventDefault() {},
  } as any)
  expect(g.cam.k).toBe(2.5)
})

test('карточка: перетаскивание двигает узел и запоминает позицию, клик без движения выбирает', () => {
  const { g, nodes, state, ev } = make()
  g.onCardDown(ev(5, 5), nodes[0] as any)
  g.onMove(ev(65, 45))
  g.onUp(ev(65, 45))
  expect(nodes[0]).toMatchObject({ x: 60, y: 40 })
  expect(state.saved.get('quest:1')).toEqual({ x: 60, y: 40 })
  expect(state.touched).toBeGreaterThan(0)
  expect(state.selected).toEqual([])
  g.onCardDown(ev(5, 5), nodes[1] as any)
  g.onUp(ev(5, 5))
  expect(state.selected).toEqual(['quest:2'])
})

test('режим связывания: клик по карточке завершает связь, а не тянет её', () => {
  const { g, nodes, state, ev } = make()
  state.linking = true
  g.onCardDown(ev(5, 5), nodes[1] as any)
  expect(state.finished).toEqual(['quest:2'])
  g.onMove(ev(110, 120))
  expect(state.linkCursor).toEqual({ x: 100, y: 100 })
})

test('выделение рамкой: попавшие в рамку карточки выбраны, клик по пустому снимает выбор', () => {
  const { g, state, ev } = make()
  state.selectMode = true
  g.onBgDown(ev(10, 20, { shiftKey: false }))
  g.onMove(ev(200, 200))
  expect(g.marquee).toMatchObject({ x0: 0, y0: 0 })
  g.onUp(ev(200, 200))
  expect([...state.picked]).toEqual(['quest:1'])
  expect(g.marquee).toBeNull()
  g.onBgDown(ev(500, 500))
  g.onUp(ev(500, 500))
  expect([...state.picked]).toEqual([])
})

test('мультиперенос: выбранные карточки двигаются вместе', () => {
  const { g, nodes, state, ev } = make()
  state.selectMode = true
  state.picked = new Set(['quest:1', 'quest:2'])
  g.onCardDown(ev(0, 0), nodes[0] as any)
  g.onMove(ev(20, 10))
  g.onUp(ev(20, 10))
  expect(nodes[0]).toMatchObject({ x: 20, y: 10 })
  expect(nodes[1]).toMatchObject({ x: 420, y: 10 })
  expect(state.saved.size).toBe(2)
})

test('щипок двумя пальцами меняет масштаб', () => {
  const { g, ev } = make()
  g.onTouchDown(ev(100, 100, { pointerType: 'touch', pointerId: 1 }))
  g.onTouchDown(ev(200, 100, { pointerType: 'touch', pointerId: 2 }))
  g.onMove(ev(100, 100, { pointerId: 1 }))
  g.onMove(ev(300, 100, { pointerId: 2 }))
  expect(g.cam.k).toBeCloseTo(2, 5)
})

test('Shift или Ctrl вне режима выделения: клик по карточке выбирает её и включает режим', () => {
  for (const mod of [{ shiftKey: true }, { ctrlKey: true }]) {
    let entered = 0
    const { g, nodes, state, ev } = make({ enterSelect: () => entered++ })
    g.onCardDown(ev(5, 5, mod), nodes[1] as any)
    g.onUp(ev(5, 5))
    expect([...state.picked]).toEqual(['quest:2'])
    expect(entered).toBe(1)
    expect(state.selected).toEqual([])
  }
})

test('Shift+рамка по полю добавляет к выбранному и включает режим; Esc-путь без модификаторов — обычная панорама', () => {
  let entered = 0
  const { g, state, ev } = make({ enterSelect: () => entered++ })
  g.onBgDown(ev(10, 20, { shiftKey: true }))
  g.onMove(ev(200, 200))
  g.onUp(ev(200, 200))
  expect([...state.picked]).toEqual(['quest:1'])
  expect(entered).toBe(1)
  const plain = make({ enterSelect: () => entered++ })
  plain.g.onBgDown(plain.ev(100, 100))
  plain.g.onMove(plain.ev(130, 90))
  plain.g.onUp(plain.ev(130, 90))
  expect(entered).toBe(1)
})
