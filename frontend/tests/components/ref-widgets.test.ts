import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { buildRefIcons, buildRefPreview } from '@/app/model/refLabels.ts'
import { bindRefRegistries } from '@/app/model/refRegistry.svelte.ts'
import { refIconStore } from '@/shared/lib/refIcons.svelte.ts'
import { plainExcerpt, setRefPreviewProvider } from '@/shared/lib/refPreview.ts'
import { scriptEmbed } from '@/shared/lib/scriptEmbed.ts'
import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'
import RefText from '@/shared/ui/RefText.svelte'

import RegistryHost from '../fixtures/refs/RegistryHost.svelte'

// Ссылки в тексте: иконка сущности, превью по hover и живая вставка script=N{…}. Всё через настоящие MarkdownBody и RefText.
let cleanup: Array<() => void> = []
afterEach(() => {
  cleanup.forEach((fn) => fn())
  cleanup = []
  refIconStore.map = {}
  setRefPreviewProvider(null)
  document.querySelectorAll('.ref-preview').forEach((n) => n.remove())
})

function body(source: string, props: Record<string, unknown> = {}) {
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(MarkdownBody, { target, props: { source, ...props } })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  flushSync()
  return target
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const hover = (el: Element) => {
  const e = new Event('pointerover', { bubbles: true }) as any
  e.pointerType = 'mouse'
  el.dispatchEvent(e)
}

test('the widgets are plain exports the shared components wire in', () => {
  expect(typeof bindRefRegistries).toBe('function')
  expect(typeof scriptEmbed).toBe('function')
})

test('plainExcerpt keeps the text and drops the markup', () => {
  const md =
    '# Заголовок\n**жирный** и `код`, 2*(2к6)+4, [ссылка](http://x) script=2{5+3к8} ![рис](attachment=1)\n```js\nskip()\n```'
  expect(plainExcerpt(md)).toBe('Заголовок жирный и код, 2*(2к6)+4, ссылка 5+3к8')
  expect(plainExcerpt('a'.repeat(400), 10)).toBe('aaaaaaaaaa…')
})

test('the registries follow the catalogs: icons and the preview card of an entity', () => {
  const target = document.createElement('div')
  document.body.append(target)
  const notes = [
    { id: 5, title: 'Лор', description: 'Тут живёт note=6.', icon: 'folder' },
    { id: 6, title: 'Город', description: 'x', is_private: true },
  ]
  const app = mount(RegistryHost, {
    target,
    props: {
      notes,
      quests: [],
      questlines: [{ id: 3, title: 'Линия', icon: 'document', icon_url: '/api/questlines/3/icon' }],
      labels: { 'note:6': 'Город' },
    },
  })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  flushSync()
  expect(refIconStore.map['note:5']).toEqual({ icon: 'folder', url: undefined })
  expect(refIconStore.map['questline:3'].url).toBe('/api/questlines/3/icon')
  expect(buildRefIcons({ notes: [], quests: [{ id: 1, questline_icon: 'x' }], questlines: [] })['quest:1'].icon).toBe(
    'x'
  )
  const card = buildRefPreview({ notes, quests: [], questlines: [] })
  expect(card('note', 5)?.body).toBe('Тут живёт note=6.')
  expect(card('note', 6)?.meta).toEqual(['приватная'])
  expect(card('note', 6)?.body).toBeUndefined()
  expect(card('note', 99)).toBeNull()
})

test('a link gets the icon of its entity; the default icon is left out', () => {
  refIconStore.map = { 'note:5': { url: '/api/notes/5/icon' }, 'note:6': { icon: 'document' } }
  const el = body('Смотри note=5 и note=6.', { labels: { 'note:5': 'Лор', 'note:6': 'Город' } })
  const links = [...el.querySelectorAll('a')]
  expect(links.map((a) => a.textContent)).toEqual(['Лор', 'Город'])
  expect(links[0].querySelector('.md-ref__ico')).not.toBeNull()
  expect(links[0].querySelector('.md-ref__ico')?.getAttribute('aria-hidden')).toBe('true')
  expect(links[1].querySelector('.md-ref__ico')).toBeNull()
  // The store changing later updates the open text.
  refIconStore.map = {}
  flushSync()
  expect(links[0].querySelector('.md-ref__ico')).toBeNull()
})

test('RefText gets icons and previews as well', async () => {
  refIconStore.map = { 'quest:2': { url: '/api/questlines/1/icon' } }
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RefText, { target, props: { source: 'quest=2', labels: { 'quest:2': 'Квест' } } })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  flushSync()
  // happy-dom's DOMPurify drops the links RefText renders, so the link is put in by hand: the action must pick up what appears later.
  target.querySelector('span')!.innerHTML = '<a class="md-ref" href="?quest=2">Квест</a>'
  await sleep(20)
  expect(target.querySelector('a .md-ref__ico')).not.toBeNull()
})

test('hover shows a card after a delay, its links open entities, a click before the delay shows nothing', async () => {
  setRefPreviewProvider(
    (kind, id) =>
      kind === 'note' && id === 2
        ? { kind, type: 'Заметка', title: 'Город', meta: ['закреплена'], body: 'Тут живёт note=3.' }
        : null,
    (k) => (k === 'note:3' ? 'Лор' : undefined)
  )
  const opened: string[] = []
  const el = body('См. note=2 и note=9.', {
    labels: { 'note:2': 'Город' },
    onRef: (k: string, id: number) => opened.push(`${k}=${id}`),
  })
  const a = el.querySelector('a') as HTMLAnchorElement

  // A click before the delay: no card appears afterwards.
  hover(a)
  a.click()
  await sleep(450)
  expect(document.querySelector('.ref-preview')).toBeNull()
  expect(opened).toEqual(['note=2']) // the click itself still opens the entity

  // Hover and wait: the card has the title, the flags and the text with a working link.
  hover(a)
  await sleep(450)
  const card = document.querySelector('.ref-preview') as HTMLElement
  expect(card.textContent).toContain('Заметка')
  expect(card.textContent).toContain('закреплена')
  const inner = card.querySelector('.ref-preview__body a') as HTMLAnchorElement
  expect(inner.textContent).toBe('Лор')
  inner.click()
  expect(opened).toEqual(['note=2', 'note=3'])
  expect(document.querySelector('.ref-preview')).toBeNull()

  // «открыть →» opens the entity the card is about; Esc closes the card; a link with no entity shows none.
  hover(a)
  await sleep(450)
  ;(document.querySelector('.ref-preview__open') as HTMLButtonElement).click()
  expect(opened).toEqual(['note=2', 'note=3', 'note=2'])
  hover(a)
  await sleep(450)
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  expect(document.querySelector('.ref-preview')).toBeNull()
  const missing = el.querySelectorAll('a')[1]
  hover(missing)
  await sleep(450)
  expect(document.querySelector('.ref-preview')).toBeNull()
})

test('a card for a link that left the page is dropped by the watchdog and by a press outside', async () => {
  setRefPreviewProvider((kind) => ({ kind, type: 'Заметка', title: 'T', meta: [] }))
  const el = body('note=2')
  const a = el.querySelector('a') as HTMLAnchorElement
  hover(a)
  await sleep(450)
  expect(document.querySelector('.ref-preview')).not.toBeNull()
  document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  expect(document.querySelector('.ref-preview')).toBeNull()
  hover(a)
  await sleep(450)
  expect(document.querySelector('.ref-preview')).not.toBeNull()
  a.remove()
  await sleep(550)
  expect(document.querySelector('.ref-preview')).toBeNull()
})

test('script=N{…} is a live link: a click computes, a second click throws again, an error says why', async () => {
  const calls: string[] = []
  let fail = false
  const realFetch = globalThis.fetch
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push(`${url} ${init.body}`)
    if (fail)
      return new Response(JSON.stringify({ detail: 'нода 2: деление на ноль' }), {
        status: 422,
        headers: { 'Content-Type': 'application/json' },
      })
    return new Response(JSON.stringify({ value: String(19 + calls.length), version: 1 }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as any
  cleanup.push(() => {
    globalThis.fetch = realFetch
  })
  const el = body('Удар script=2{5+3к8} урона.')
  const a = el.querySelector('a.md-script') as HTMLAnchorElement
  expect(a.textContent).toBe('5+3к8')
  a.click()
  await sleep(50)
  expect(a.textContent).toBe('5+3к8 = 20')
  expect(calls[0]).toBe('/api/scripts/2/eval {"args":{"_":"5+3к8"}}')
  a.click()
  await sleep(50)
  expect(a.textContent).toBe('5+3к8 = 21')
  fail = true
  a.click()
  await sleep(50)
  expect(a.textContent).toBe('5+3к8 — ошибка')
  expect(a.classList.contains('md-script--err')).toBe(true)
  expect(a.title).toContain('деление на ноль')
})
