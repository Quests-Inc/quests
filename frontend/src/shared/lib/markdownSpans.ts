// span-id pairs and note=N#key quotes. Comments stay hidden; a valid pair becomes data-md-span.

import { marked } from 'marked'

import { getNote } from '@/shared/api/client.ts'

import { findMarkdownTarget } from './markdownTargets.ts'

const KEY = '[a-z][a-z0-9_-]{0,63}'
const COMMENT = new RegExp(`<!--\\s*(\\/?)span-id:\\s*(${KEY})\\s*-->`, 'g')

type Hit = { close: boolean; key: string; start: number; end: number }

function spanHits(source: string): Hit[] {
  const hits: Hit[] = []
  let fence: string | null = null
  let offset = 0
  for (const line of source.split('\n')) {
    const opened = /^\s*(```+|~~~+)/.exec(line)
    if (fence) {
      if (opened && opened[1].startsWith(fence[0]) && opened[1].length >= fence.length) fence = null
    } else if (opened) fence = opened[1]
    else {
      COMMENT.lastIndex = 0
      let m: RegExpExecArray | null
      while ((m = COMMENT.exec(line))) {
        hits.push({ close: m[1] === '/', key: m[2], start: offset + m.index, end: offset + m.index + m[0].length })
      }
    }
    offset += line.length + 1
  }
  return hits
}

/** Content between a well-formed pair. A missing close, a foreign close, a repeated key or a cross returns nothing for that key. */
export function spanRanges(source: string): Map<string, { start: number; end: number }> {
  const hits = spanHits(source)
  const stack: Hit[] = []
  const seen = new Set<string>()
  const bad = new Set<string>()
  const ranges = new Map<string, { start: number; end: number }>()
  for (const hit of hits) {
    if (!hit.close) {
      if (seen.has(hit.key)) bad.add(hit.key)
      seen.add(hit.key)
      stack.push(hit)
      continue
    }
    const top = stack.at(-1)
    if (top?.key === hit.key) {
      stack.pop()
      ranges.set(hit.key, { start: top.end, end: hit.start })
      continue
    }
    bad.add(hit.key)
    if (top) bad.add(top.key)
    const idx = stack.findIndex((open) => open.key === hit.key)
    if (idx >= 0) {
      for (const open of stack.slice(idx)) bad.add(open.key)
      stack.splice(idx)
    }
  }
  for (const open of stack) bad.add(open.key)
  for (const key of bad) ranges.delete(key)
  return ranges
}

function oneLine(text: string) {
  const flat = text.replace(/\s+/g, ' ').trim()
  if (!flat) return null
  return flat.length <= 180 ? flat : `${flat.slice(0, 179).trimEnd()}…`
}

/** Quote is the readable text, so a heading is «Свет», not «## Свет». */
function plainQuote(source: string) {
  const html = marked.parse(source, { async: false })
  if (typeof html !== 'string' || typeof document === 'undefined') return oneLine(source)
  const doc = document.createElement('div')
  doc.innerHTML = html
  const blocks = [...doc.querySelectorAll('h1,h2,h3,h4,h5,h6,p,li,blockquote,pre')]
  const text = blocks.length ? blocks.map((n) => n.textContent || '').join(' ') : doc.textContent || ''
  return oneLine(text)
}

export function spanQuote(source: string, key: string) {
  const range = spanRanges(source).get(key)
  if (!range) return null
  return plainQuote(source.slice(range.start, range.end))
}

/** Plain text of one body row. A missing row, a repeated row-id or a repeated table-id is unavailable. */
export function rowQuote(source: string, table: string, row: string) {
  let fence: string | null = null
  let active = false
  let tables = 0
  const seen = new Set<string>()
  let quote: string | null = null
  for (const line of source.split('\n')) {
    const opened = /^\s*(```+|~~~+)/.exec(line)
    if (fence) {
      if (opened && opened[1].startsWith(fence[0]) && opened[1].length >= fence.length) fence = null
      continue
    }
    if (opened) {
      fence = opened[1]
      continue
    }
    const id = /^\s*<!--\s*table-id:\s*([a-z][a-z0-9_-]{0,63})\s*-->\s*$/.exec(line)
    if (id) {
      active = id[1] === table
      if (active) tables++
      continue
    }
    if (!active || !line.trim().startsWith('|') || /^\|\s*-+/.test(line.trim())) continue
    const mark = new RegExp(`<!--\\s*row-id:\\s*(${KEY})\\s*-->`).exec(line)
    if (!mark) continue
    if (seen.has(mark[1])) return null
    seen.add(mark[1])
    if (mark[1] !== row) continue
    const cells = line
      .replace(new RegExp(`<!--\\s*row-id:\\s*${KEY}\\s*-->`), '')
      .split('|')
      .map((cell) => cell.trim())
      .filter(Boolean)
    quote = plainQuote(cells.join(' · '))
  }
  if (tables !== 1) return null
  return quote
}

export function fragmentQuote(source: string, fragment: string) {
  const cut = fragment.indexOf('/')
  if (cut < 0) return spanQuote(source, fragment)
  return rowQuote(source, fragment.slice(0, cut), fragment.slice(cut + 1))
}

export function fragmentMissLabel(fragment: string) {
  return fragment.includes('/') ? 'Строка недоступна' : 'Кусок недоступен'
}

const noteText = new Map<number, string>()

function setLinkText(a: HTMLAnchorElement, text: string) {
  const icon = a.querySelector(':scope > .md-ref__ico')
  a.textContent = text
  if (icon) a.prepend(icon)
}

/** Scroll to a span or a table row in the rendered note. */
export function scrollFragment(root: HTMLElement, fragment: string) {
  const cut = fragment.indexOf('/')
  let el: HTMLElement | null = null
  if (cut > 0) {
    el = findMarkdownTarget(root, { table: fragment.slice(0, cut), row: fragment.slice(cut + 1) })
  } else {
    const found = [...root.querySelectorAll<HTMLElement>('[data-md-span]')].filter((n) => n.dataset.mdSpan === fragment)
    el = found.length === 1 ? found[0] : null
  }
  if (!el) return false
  const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  el.scrollIntoView?.({ block: 'center', behavior: reduce ? 'instant' : 'smooth' })
  el.classList.add('md-target--flash')
  window.setTimeout(() => el?.classList.remove('md-target--flash'), 1600)
  return true
}

async function textOf(id: number, hostId: number | null, source: string) {
  if (hostId === id) return source
  const cached = noteText.get(id)
  if (cached != null) return cached
  try {
    const row = await getNote(id)
    const text = row.description || ''
    noteText.set(id, text)
    return text
  } catch {
    return null
  }
}

/** The quoted piece is the link text. No note title and no quotation marks. */
export function refQuotes(node: HTMLElement, initial: { source: string; noteId: number | null; html?: string }) {
  let spec = initial
  let generation = 0
  let sig = ''
  const run = () => {
    const next = `${spec.noteId}\0${spec.source}\0${spec.html ?? ''}`
    if (next === sig) return
    sig = next
    const mine = ++generation
    for (const a of node.querySelectorAll<HTMLAnchorElement>('a[href]')) {
      const href = a.getAttribute('href') || ''
      const hash = href.includes('#') ? decodeURIComponent(href.slice(href.indexOf('#') + 1)) : ''
      if (!hash || !/^[a-z][a-z0-9_-]{0,63}(?:\/[a-z][a-z0-9_-]{0,63})?$/.test(hash)) continue
      const id = Number(new URL(href, 'http://local').searchParams.get('note'))
      if (!Number.isInteger(id) || id <= 0) continue
      void textOf(id, spec.noteId, spec.source).then((text) => {
        if (mine !== generation || !a.isConnected) return
        const quote = text == null ? null : fragmentQuote(text, hash)
        a.classList.toggle('md-ref--miss', !quote)
        setLinkText(a, quote || fragmentMissLabel(hash))
      })
    }
  }
  run()
  return {
    update(next: { source: string; noteId: number | null; html?: string }) {
      spec = next
      run()
    },
    destroy() {
      generation++
    },
  }
}

function commentMark(data: string): { close: boolean; key: string } | null {
  const open = new RegExp(`^\\s*span-id:\\s*(${KEY})\\s*$`).exec(data)
  if (open) return { close: false, key: open[1] }
  const close = new RegExp(`^\\s*/span-id:\\s*(${KEY})\\s*$`).exec(data)
  if (close) return { close: true, key: close[1] }
  return null
}

/** Wrap each valid pair. Comments themselves are removed, so preview shows only the piece. */
export function markSpanTargets(dirty: string): string {
  if (!dirty.includes('span-id:')) return dirty
  const doc = window.document.implementation.createHTMLDocument('')
  doc.body.innerHTML = dirty
  const comments: Comment[] = []
  const walk = doc.createTreeWalker(doc.body, 128)
  while (walk.nextNode()) comments.push(walk.currentNode as Comment)
  const marks = comments.flatMap((node) => {
    const mark = commentMark(node.data)
    return mark ? [{ node, ...mark }] : []
  })
  const stack: { node: Comment; key: string }[] = []
  const seen = new Set<string>()
  const bad = new Set<string>()
  const pairs: { open: Comment; close: Comment; key: string }[] = []
  for (const mark of marks) {
    if (!mark.close) {
      if (seen.has(mark.key)) bad.add(mark.key)
      seen.add(mark.key)
      stack.push(mark)
      continue
    }
    const top = stack.at(-1)
    if (top?.key === mark.key) {
      stack.pop()
      pairs.push({ open: top.node, close: mark.node, key: mark.key })
      continue
    }
    bad.add(mark.key)
    if (top) bad.add(top.key)
    const idx = stack.findIndex((open) => open.key === mark.key)
    if (idx >= 0) {
      for (const open of stack.slice(idx)) bad.add(open.key)
      stack.splice(idx)
    }
  }
  for (const open of stack) bad.add(open.key)
  for (const pair of pairs) {
    if (bad.has(pair.key)) continue
    if (pair.open.parentNode !== pair.close.parentNode || !pair.open.parentNode) continue
    const parent = pair.open.parentNode
    const inline =
      parent instanceof HTMLElement && /^(P|SPAN|A|LI|TD|TH|H[1-6]|BLOCKQUOTE|EM|STRONG|DEL)$/.test(parent.tagName)
    const box = doc.createElement(inline ? 'span' : 'div')
    box.setAttribute('data-md-span', pair.key)
    parent.insertBefore(box, pair.open)
    let node: ChildNode | null = pair.open
    while (node) {
      const next: ChildNode | null = node.nextSibling
      box.appendChild(node)
      if (node === pair.close) break
      node = next
    }
    pair.open.remove()
    pair.close.remove()
  }
  return doc.body.innerHTML
}
