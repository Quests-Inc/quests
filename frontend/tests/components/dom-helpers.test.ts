import { afterEach, expect, test } from 'bun:test'

import { foldRow, moveRow } from '@/shared/lib/listNav.ts'
import { downloadPdf } from '@/shared/lib/pdfExport.ts'
import { placePopover, watchPopover } from '@/shared/lib/popover.ts'
import { animateText } from '@/shared/lib/typewriter.ts'

afterEach(() => {
  document.body.innerHTML = ''
})

function box(rect: Partial<DOMRect>, size = { w: 100, h: 50 }) {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => ({ left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0, ...rect }) as DOMRect
  Object.defineProperty(el, 'offsetWidth', { value: size.w })
  Object.defineProperty(el, 'offsetHeight', { value: size.h })
  return el
}

test('popover: под якорем, а без места снизу — над ним; края окна держит', () => {
  window.innerWidth = 1000
  window.innerHeight = 600
  const pop = box({}, { w: 200, h: 100 })
  expect(placePopover(box({ left: 50, top: 20, right: 150, bottom: 40, width: 100 }), pop, { matchWidth: true })).toBe(
    'below'
  )
  expect([pop.style.left, pop.style.top, pop.style.minWidth]).toEqual(['50px', '44px', '100px'])
  expect(placePopover(box({ left: 950, top: 560, right: 990, bottom: 580, width: 40 }), pop)).toBe('above')
  expect([pop.style.left, pop.style.top]).toEqual(['792px', '456px'])
  placePopover(box({ left: 300, top: 20, right: 500, bottom: 40, width: 200 }), pop, { align: 'end' })
  expect(pop.style.left).toBe('300px')
})

test('watchPopover: клик снаружи закрывает, внутри — нет; прокрутка самого списка не двигает; cleanup снимает слушатели', () => {
  const inside = document.createElement('div')
  const list = document.createElement('div')
  inside.append(list)
  document.body.append(inside)
  const log: string[] = []
  const stop = watchPopover(
    () => [inside],
    () => log.push('close'),
    () => log.push('move')
  )
  const down = (target: Element) => target.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  down(document.body)
  down(list)
  expect(log).toEqual(['close'])
  list.dispatchEvent(new Event('scroll', { bubbles: true }))
  document.body.dispatchEvent(new Event('scroll'))
  window.dispatchEvent(new Event('resize'))
  expect(log).toEqual(['close', 'move'].concat(['move']).slice(0, 3))
  stop()
  down(document.body)
  expect(log).toHaveLength(3)
})

test('listNav: ходьба по видимым строкам и складывание группы', () => {
  document.body.innerHTML = `<div data-nav-root><div data-nav-node><button aria-expanded="true" data-nav id="a">a</button></div><button data-nav id="b" class="notes__row--on">b</button><button data-nav id="c">c</button></div>`
  const rows = [...document.querySelectorAll<HTMLElement>('[data-nav]')]
  for (const r of rows) r.getClientRects = () => [{}] as unknown as DOMRectList
  expect(moveRow(1)).toBe(true)
  expect(document.activeElement?.id).toBe('b')
  moveRow(1)
  expect(document.activeElement?.id).toBe('c')
  moveRow(1)
  expect(document.activeElement?.id).toBe('c')
  moveRow(-1)
  expect(document.activeElement?.id).toBe('b')
  rows[0].focus()
  let clicks = 0
  rows[0].addEventListener('click', () => clicks++)
  expect(foldRow(true)).toBe(false)
  expect(foldRow(false)).toBe(true)
  expect(clicks).toBe(1)
  document.body.innerHTML = ''
  expect(moveRow(1)).toBe(false)
})

test('typewriter: слова проявляются по порядку, стирание идёт с конца, abort мгновенно завершает', async () => {
  window.matchMedia = (() => ({ matches: false })) as any
  const parts: string[] = []
  await animateText('раз два три', { granularity: 'word', stepMs: 1, onUpdate: (p) => parts.push(p) })
  expect(parts.slice(0, 3)).toEqual(['раз ', 'раз два ', 'раз два три'])
  expect(parts.at(-1)).toBe('раз два три')
  const erased: string[] = []
  await animateText('аб', { mode: 'erase', stepMs: 1, onUpdate: (p) => erased.push(p) })
  expect(erased).toEqual(['а', '', ''])
  const ctrl = new AbortController()
  const aborted: string[] = []
  const run = animateText('длинный текст', { stepMs: 50, onUpdate: (p) => aborted.push(p), signal: ctrl.signal })
  ctrl.abort()
  await run
  expect(aborted.at(-1)).toBe('длинный текст')
  window.matchMedia = (() => ({ matches: true })) as any
  const instant: string[] = []
  await animateText('быстро', { onUpdate: (p) => instant.push(p) })
  expect(instant).toEqual(['быстро'])
})

test('pdfExport: отправляет документ на сервер и скачивает полученный файл', async () => {
  const sent: any[] = []
  const realFetch = globalThis.fetch
  globalThis.fetch = (async (url: string, init: any) => {
    sent.push([url, JSON.parse(init.body)])
    return new Response(new Blob(['%PDF']), { status: 200 })
  }) as any
  const downloads: string[] = []
  ;(URL as any).createObjectURL = () => 'blob:x'
  ;(URL as any).revokeObjectURL = () => {}
  const realCreate = document.createElement.bind(document)
  document.createElement = ((tag: string) => {
    const el = realCreate(tag)
    if (tag === 'a') el.click = () => downloads.push((el as HTMLAnchorElement).download)
    return el
  }) as any
  try {
    await downloadPdf({ title: 'Т', footnote: '', labels: {}, blocks: [] }, 'a.pdf')
    expect(sent).toEqual([
      ['/api/export/pdf', { document: { title: 'Т', footnote: '', labels: {}, blocks: [] }, filename: 'a.pdf' }],
    ])
    expect(downloads).toEqual(['a.pdf'])
    globalThis.fetch = (async () => new Response('нет шрифта', { status: 500 })) as any
    await expect(downloadPdf({ title: 'Т', footnote: '', labels: {}, blocks: [] }, 'a.pdf')).rejects.toThrow(
      'PDF render failed (500): нет шрифта'
    )
  } finally {
    globalThis.fetch = realFetch
    document.createElement = realCreate
  }
})
