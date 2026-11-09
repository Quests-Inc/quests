import { renderToString } from 'katex'
import type { TokenizerExtension, RendererExtension } from 'marked'

/** No whitespace next to inline delimiters; a closing dollar before a digit is currency. */
export function readMath(src: string, block = false) {
  if (src.startsWith('$$')) {
    const end = src.indexOf('$$', 2)
    if (end < 0) return undefined
    const raw = src.slice(0, end + 2)
    return { type: 'math', raw, text: raw.slice(2, -2).trim(), display: true }
  }
  if (block || !src.startsWith('$') || /^\$\s/.test(src)) return undefined
  const m = /^\$((?:\\.|[^$\n])+?)\$(?!\d)/.exec(src)
  if (!m || /\s$/.test(m[1])) return undefined
  return { type: 'math', raw: m[0], text: m[1], display: false }
}
const escape = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const renderer: RendererExtension['renderer'] = (token) =>
  `<span data-math="${escape(encodeURIComponent(token.text))}" data-display="${token.display ? '1' : '0'}"></span>`
export const mathExtensions: (TokenizerExtension & RendererExtension)[] = [
  {
    name: 'math',
    level: 'block',
    start: (src) => src.indexOf('$$'),
    tokenizer: (src) => readMath(src, true),
    renderer,
  },
  { name: 'math', level: 'inline', start: (src) => src.indexOf('$'), tokenizer: (src) => readMath(src), renderer },
]
/** Only library-generated HTML bypasses the prose sanitizer. User HTML keeps the narrow allowlist. */
export function renderMathElements(html: string): string {
  if (!html.includes('data-math=')) return html
  const doc = window.document.implementation.createHTMLDocument('')
  doc.body.innerHTML = html
  let count = 0
  for (const el of doc.querySelectorAll<HTMLElement>('[data-math]')) {
    let tex = el.dataset.math ?? ''
    try {
      tex = decodeURIComponent(tex)
    } catch {
      /* raw user attributes remain literal */
    }
    const display = el.dataset.display === '1'
    el.removeAttribute('data-math')
    el.removeAttribute('data-display')
    el.className = display ? 'md-math md-math--display' : 'md-math'
    try {
      if (++count > 200 || tex.length > 4096) throw new Error('Слишком большая формула')
      el.innerHTML = renderToString(tex, {
        displayMode: display,
        throwOnError: true,
        trust: false,
        strict: 'ignore',
        maxExpand: 200,
        maxSize: 20,
        output: 'htmlAndMathml',
      })
    } catch {
      el.classList.add('md-math--error')
      el.textContent = `${display ? '$$' : '$'}${tex}${display ? '$$' : '$'}`
      el.title = 'Не удалось отобразить формулу'
    }
  }
  return doc.body.innerHTML
}
