// Wiki-превью при наведении на реф note=/quest=/questline=. Один попап на документ, рефы внутри него — не ссылки (без каскада).
// Данные берутся из каталогов в памяти через provider (App), поэтому лишних запросов нет.
import { parseRefHref, REF_RE } from './refs.ts'

export type RefPreview = {
  kind: string
  title: string
  /** «Заметка» / «Квест» / «Квестлайн». */
  type: string
  /** Короткие флаги и поля в одну строку. */
  meta: string[]
  /** Плоский текст, уже урезанный. */
  body?: string
}

type Provider = (kind: string, id: number) => RefPreview | null
let provider: Provider | null = null
let labelOf: (key: string) => string | undefined = () => undefined
export const setRefPreviewProvider = (p: Provider | null, labels?: (key: string) => string | undefined) => {
  provider = p
  labelOf = labels ?? (() => undefined)
}

const SHOW_MS = 350
const HIDE_MS = 150

/** Markdown → плоский текст для превью (картинки, код и разметка убираются). */
export function plainExcerpt(md: string, max = 320) {
  const t = String(md || '')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\bscript=\d+\{([^{}\n]*)\}/g, '$1')
    .replace(/(\*\*|__)(.+?)\1/g, '$2')
    .replace(/^[>#\s]+/gm, '')
    .replace(/[`~|]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  return t.length > max ? `${t.slice(0, max).trimEnd()}…` : t
}

let card: HTMLDivElement | null = null
let anchor: HTMLAnchorElement | null = null
let showTimer = 0
let hideTimer = 0
let watchdog = 0

function close() {
  clearTimeout(showTimer)
  clearTimeout(hideTimer)
  clearInterval(watchdog)
  card?.remove()
  card = null
  anchor = null
}

function scheduleClose() {
  clearTimeout(showTimer)
  clearTimeout(hideTimer)
  hideTimer = window.setTimeout(close, HIDE_MS)
}

function place(el: HTMLElement, a: HTMLElement) {
  const r = a.getBoundingClientRect()
  const w = el.offsetWidth
  const h = el.offsetHeight
  const gap = 6
  let top = r.bottom + gap
  if (top + h > window.innerHeight - 8 && r.top - gap - h > 8) top = r.top - gap - h
  const left = Math.max(8, Math.min(r.left, window.innerWidth - w - 8))
  el.style.left = `${left}px`
  el.style.top = `${Math.max(8, top)}px`
}

/** Plain text with note=/quest=/… tokens made into links (no preview of their own: no cascade). */
function appendWithRefs(
  into: HTMLElement,
  text: string,
  origin: HTMLAnchorElement,
  onRef?: (kind: string, id: number) => void
) {
  let last = 0
  for (const m of text.matchAll(new RegExp(REF_RE.source, REF_RE.flags))) {
    const idx = m.index ?? 0
    if (idx > last) into.append(text.slice(last, idx))
    const a = document.createElement('a')
    a.className = 'md-ref'
    a.href = `?${m[1]}=${m[2]}`
    a.textContent = labelOf(`${m[1]}:${m[2]}`) || m[0]
    a.addEventListener('click', (e) => {
      e.preventDefault()
      e.stopPropagation()
      close()
      if (onRef) onRef(m[1], Number(m[2]))
      else origin.click()
    })
    into.append(a)
    last = idx + m[0].length
  }
  if (last < text.length) into.append(text.slice(last))
}

function open(a: HTMLAnchorElement, kind: string, id: number, onRef?: (kind: string, id: number) => void) {
  const p = provider?.(kind, id)
  close()
  // The link may be gone by now (a click navigated away while the delay ran): a card for it would land in a corner.
  if (!p || !a.isConnected) return
  const el = document.createElement('div')
  el.className = 'ref-preview'
  el.setAttribute('role', 'tooltip')
  const head = document.createElement('div')
  head.className = 'ref-preview__head'
  const type = document.createElement('span')
  type.className = 'ref-preview__type'
  type.textContent = p.type
  const title = document.createElement('strong')
  title.textContent = p.title
  head.append(type, title)
  el.append(head)
  if (p.meta.length) {
    const m = document.createElement('div')
    m.className = 'ref-preview__meta'
    m.textContent = p.meta.join(' · ')
    el.append(m)
  }
  if (p.body) {
    const b = document.createElement('div')
    b.className = 'ref-preview__body'
    appendWithRefs(b, p.body, a, onRef)
    el.append(b)
  }
  const go = document.createElement('button')
  go.type = 'button'
  go.className = 'ref-preview__open'
  go.textContent = 'открыть →'
  go.addEventListener('click', () => {
    const target = a
    close()
    if (onRef) onRef(kind, id)
    else target.click()
  })
  el.append(go)
  el.addEventListener('pointerenter', () => clearTimeout(hideTimer))
  el.addEventListener('pointerleave', scheduleClose)
  el.style.visibility = 'hidden'
  document.body.append(el)
  place(el, a)
  el.style.visibility = ''
  card = el
  anchor = a
  // A link removed under an open card never sends pointerout; don't leave the card hanging.
  watchdog = window.setInterval(() => {
    if (!anchor?.isConnected) close()
  }, 400)
}

/** Svelte action: превью по hover (только мышь) и focus-visible, Esc закрывает. Тач — без превью, остаётся обычный клик. */
export function refPreview(node: HTMLElement, initial?: (kind: string, id: number) => void) {
  let onRef = initial
  const refOf = (t: EventTarget | null) => {
    const a = t instanceof Element ? (t.closest('a[href]') as HTMLAnchorElement | null) : null
    if (!a || !node.contains(a)) return null
    const ref = parseRefHref(a.getAttribute('href') || '')
    return ref && ['note', 'quest', 'questline'].includes(ref.kind) ? { a, ...ref } : null
  }
  const arm = (r: NonNullable<ReturnType<typeof refOf>>, delay: number) => {
    clearTimeout(hideTimer)
    if (anchor === r.a) return
    clearTimeout(showTimer)
    showTimer = window.setTimeout(() => open(r.a, r.kind, r.id, onRef), delay)
  }
  const onOver = (e: PointerEvent) => {
    if (e.pointerType !== 'mouse') return
    const r = refOf(e.target)
    if (r) arm(r, SHOW_MS)
  }
  const onOut = (e: PointerEvent) => {
    if (e.pointerType !== 'mouse' || !refOf(e.target)) return
    scheduleClose()
  }
  const onFocus = (e: FocusEvent) => {
    const r = refOf(e.target)
    if (r && (e.target as Element).matches(':focus-visible')) arm(r, 0)
  }
  const onBlur = (e: FocusEvent) => {
    if (refOf(e.target)) scheduleClose()
  }
  const onClickRef = (e: MouseEvent) => {
    if (refOf(e.target)) close()
  }
  const onDocDown = (e: PointerEvent) => {
    if (card && !(e.target instanceof Node && card.contains(e.target))) close()
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && card) close()
  }
  node.addEventListener('pointerover', onOver)
  node.addEventListener('pointerout', onOut)
  node.addEventListener('focusin', onFocus)
  node.addEventListener('focusout', onBlur)
  node.addEventListener('click', onClickRef, true)
  document.addEventListener('pointerdown', onDocDown, true)
  document.addEventListener('keydown', onKey)
  return {
    update(next?: (kind: string, id: number) => void) {
      onRef = next
    },
    destroy() {
      node.removeEventListener('pointerover', onOver)
      node.removeEventListener('pointerout', onOut)
      node.removeEventListener('focusin', onFocus)
      node.removeEventListener('focusout', onBlur)
      node.removeEventListener('click', onClickRef, true)
      document.removeEventListener('pointerdown', onDocDown, true)
      document.removeEventListener('keydown', onKey)
      if (anchor && node.contains(anchor)) close()
    },
  }
}
