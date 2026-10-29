import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { highlightCode } from './codeHighlight.ts'
import { REF_RE, refHref } from './refs.ts'
import { escapeMdInline, scriptEmbedRe, scriptFlags, scriptHref, scriptLabel } from './scriptRef.ts'

/** Canonical language id for CSS / data-lang (aliases → primary). */
const LANG_CANON: Record<string, string> = {
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  js: 'javascript',
  ts: 'typescript',
  py: 'python',
  rs: 'rust',
  golang: 'go',
  docker: 'dockerfile',
  yml: 'yaml',
  md: 'markdown',
  html: 'xml',
  svg: 'xml',
  toml: 'ini',
}

marked.setOptions({ gfm: true, breaks: true })

marked.use({
  renderer: {
    code({ text, lang }) {
      const rawLang = String(lang || '')
        .trim()
        .split(/\s+/)[0]
        .toLowerCase()
      const canon = LANG_CANON[rawLang] || rawLang
      const highlighted = highlightFence(text, canon || rawLang)
      const langAttr = canon ? ` data-lang="${escapeAttr(canon)}"` : ''
      const classAttr = canon ? ` class="hljs language-${escapeAttr(canon)}"` : ' class="hljs"'
      return `<div class="md-codeblock"${langAttr}><pre><code${classAttr}>${highlighted}</code></pre></div>\n`
    },
  },
})

function highlightFence(code: string, lang: string) {
  return highlightCode(String(code ?? '').replace(/\n$/, ''), lang)
}

function escapeAttr(s: any) {
  return String(s).replace(/[^a-z0-9_+-]/gi, '')
}

const PURIFY_OPTS = {
  ALLOWED_TAGS: [
    'p',
    'br',
    'ul',
    'ol',
    'li',
    'strong',
    'em',
    'code',
    'pre',
    'a',
    'img',
    'h1',
    'h2',
    'h3',
    'h4',
    'blockquote',
    'hr',
    'table',
    'thead',
    'tbody',
    'tfoot',
    'tr',
    'th',
    'td',
    'div',
    'span',
  ],
  ALLOWED_ATTR: ['href', 'rel', 'target', 'src', 'alt', 'align', 'class', 'scope'],
  ADD_ATTR: ['data-lang'],
  ALLOW_DATA_ATTR: false,
}

const TABLE_SIDE_RE = /^\s*table:\s*(top|left|both|none)\s*$/i

/** Which cells of a table are headers. */
export type TableSide = 'top' | 'left' | 'both' | 'none'

function asCell(from: Element, tag: 'td' | 'th', scope = ''): HTMLElement {
  const cell = from.ownerDocument.createElement(tag)
  cell.innerHTML = from.innerHTML
  const align = from.getAttribute('align')
  if (align) cell.setAttribute('align', align)
  if (scope) cell.setAttribute('scope', scope)
  return cell
}

/**
 * Pipe tables put the header on top, always. `<!-- table: left -->` on the line above a table says where the headers are:
 * `top` (the default), `left` (the first column; the top row is an ordinary row), `both`, or `none`. A table whose header
 * row is empty (`| | |`) has no top header, so it reads as `left` unless the line says otherwise. Other viewers show the
 * comment as nothing and the table as a normal one.
 */
export function shapeTables(dirty: string): string {
  if (!dirty.includes('<table')) return dirty
  const doc = window.document.implementation.createHTMLDocument('')
  doc.body.innerHTML = dirty
  for (const table of Array.from(doc.querySelectorAll('table'))) {
    let side: TableSide | '' = ''
    let prev = table.previousSibling
    while (prev && prev.nodeType === 3 && !(prev.textContent || '').trim()) prev = prev.previousSibling
    if (prev && prev.nodeType === 8) {
      const m = TABLE_SIDE_RE.exec(prev.textContent || '')
      if (m) {
        side = m[1].toLowerCase() as TableSide
        prev.remove()
      }
    }
    const head = table.tHead
    const headRow = head?.rows[0]
    const headCells = headRow ? Array.from(headRow.cells) : []
    const emptyHead = headCells.length > 0 && headCells.every((c) => !(c.textContent || '').trim())
    const use: TableSide = side || (emptyHead ? 'left' : 'top')
    const body = table.tBodies[0] ?? table.appendChild(doc.createElement('tbody'))
    if (head && (use === 'left' || use === 'none' || (use === 'both' && emptyHead))) {
      if (!emptyHead && headRow) {
        const row = doc.createElement('tr')
        for (const c of headCells) row.append(asCell(c, 'td'))
        body.insertBefore(row, body.firstChild)
      }
      head.remove()
    }
    if (use === 'top' || use === 'both') {
      for (const th of Array.from(table.querySelectorAll('thead th'))) th.setAttribute('scope', 'col')
    }
    if (use === 'left' || use === 'both') {
      for (const row of Array.from(body.rows)) {
        const first = row.cells[0]
        if (first && first.tagName === 'TD') first.replaceWith(asCell(first, 'th', 'row'))
      }
    }
  }
  return doc.body.innerHTML
}

/** The only `<img src>` the sanitizer lets through: an attachment's inline view. */
export const ATTACHMENT_IMAGE_SRC_RE = /^\/api\/attachments\/(\d+)\/image$/

/** `![alt](attachment=N)` — images come only from attachments (ClamAV-scanned at upload). */
const ATTACHMENT_IMG_RE = /!\[([^\]\n]*)\]\(\s*attachment=(\d+)\s*\)/g

export function attachmentImageSrc(id: number | string) {
  return `/api/attachments/${id}/image`
}

let hooked = false

function ensureLinkHook() {
  if (hooked || typeof window === 'undefined') return
  hooked = true
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'IMG') {
      // No external hosts, no data: URIs, no raw <img> — only attachment images.
      if (!ATTACHMENT_IMAGE_SRC_RE.test(node.getAttribute('src') || '')) {
        node.remove()
        return
      }
      // `![подпись|center](attachment=N)` — alignment is an alt-text suffix, so it
      // stays plain markdown and per-image (no global image rule).
      const m = /^(.*?)\s*\|\s*(left|center|right)\s*$/i.exec(node.getAttribute('alt') || '')
      if (m) node.setAttribute('alt', m[1])
      // Idempotent: a second sanitize pass sees the already-cleaned alt, so keep
      // an alignment class that is already there.
      const prev = /\bmd-img--(left|center|right)\b/.exec(node.getAttribute('class') || '')
      const align = m ? m[2].toLowerCase() : prev?.[1]
      node.setAttribute('class', align ? `md-attachment-img md-img--${align}` : 'md-attachment-img')
      return
    }
    if (node.tagName === 'A') {
      const href = node.getAttribute('href') || ''
      if (href.startsWith('?') || href.startsWith('/')) {
        node.removeAttribute('target')
        node.removeAttribute('rel')
        return
      }
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  })
}

function splitCode(src: string) {
  return String(src || '').split(/(```[\s\S]*?```|`[^`]+`)/)
}

const MUSTACHE_RE = /\{\{\s*([a-z][a-z0-9_]{0,23})\s*\}\}/g
const FOREIGN_MUSTACHE_RE = /\{\{\s*(note|quest|questline|step|template)=(\d+)\.([a-z][a-z0-9_]{0,23})\s*\}\}/g
const MAX_FOREIGN_VALUES = 8

export type ForeignValueRef = { key: string; kind: string; id: number; name: string }

/** `{{note=12.str}}` outside code and script braces. Eight different refs per text. */
export function foreignValueRefs(source: string): ForeignValueRef[] {
  const out: ForeignValueRef[] = []
  const seen = new Set<string>()
  const parts = splitCode(source)
  FOREIGN_MUSTACHE_RE.lastIndex = 0
  for (let i = 0; i < parts.length; i += 2) {
    FOREIGN_MUSTACHE_RE.lastIndex = 0
    const shielded = shieldScripts(parts[i] ?? '')
    for (const m of shielded.text.matchAll(FOREIGN_MUSTACHE_RE)) {
      const key = `${m[1]}=${m[2]}.${m[3]}`
      if (seen.has(key)) continue
      if (seen.size >= MAX_FOREIGN_VALUES) {
        FOREIGN_MUSTACHE_RE.lastIndex = 0
        return out
      }
      seen.add(key)
      out.push({ key, kind: m[1], id: Number(m[2]), name: m[3] })
    }
  }
  FOREIGN_MUSTACHE_RE.lastIndex = 0
  return out
}

function escapeHtmlText(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/** Chip inserted after sanitizing, so the class is ours and the text stays escaped. */
function valueChip(source: string, text: string) {
  return `<span class="md-value" data-value="${escapeHtmlText(source)}">${escapeHtmlText(text)}</span>`
}

/** Script braces are one token: a `note=12` inside them must not become its own link. */
function shieldScripts(chunk: string) {
  const saved: string[] = []
  const text = chunk.replace(scriptEmbedRe(), (full) => {
    saved.push(full)
    return `%%SCRIPT${saved.length - 1}%%`
  })
  return { text, saved }
}

function linkifyRefs(
  source: string,
  opts: { labels?: Record<string, string>; values?: Record<string, string> } = {},
  bag: string[] = []
) {
  const labels = opts.labels || {}
  const values = opts.values || {}
  const withMdLinks = splitCode(source)
    .map((part, i) => {
      if (i % 2 === 1) return part
      const shielded = shieldScripts(part)
      const held: string[] = []
      FOREIGN_MUSTACHE_RE.lastIndex = 0
      let chunk = shielded.text.replace(FOREIGN_MUSTACHE_RE, (full, kind, id, name) => {
        const key = `${kind}=${id}.${name}`
        if (Object.prototype.hasOwnProperty.call(values, key)) {
          const token = `⟦V${bag.length}⟧`
          bag.push(valueChip(`${kind}=${id}.${name}`, values[key]))
          return token
        }
        const token = `%%HOLD${held.length}%%`
        held.push(full)
        return token
      })
      chunk = chunk.replace(MUSTACHE_RE, (full, name) => {
        if (!Object.prototype.hasOwnProperty.call(values, name)) return full
        const token = `⟦V${bag.length}⟧`
        bag.push(valueChip(name, values[name]))
        return token
      })
      chunk = chunk
        .replace(ATTACHMENT_IMG_RE, (_, alt, id) => `![${alt}](${attachmentImageSrc(id)})`)
        .replace(REF_RE, (full, kind, id) => {
          const label = labels[`${kind}:${id}`] || full
          return `[${label}](${refHref(kind, id)})`
        })
        .replace(/%%SCRIPT(\d+)%%/g, (_, n) => shielded.saved[Number(n)] ?? '')
        .replace(/%%HOLD(\d+)%%/g, (_, n) => held[Number(n)] ?? '')
      // `script=12{3к8}` computes on click. `?auto` computes when shown. `?hide` shows `?` until then.
      return chunk.replace(scriptEmbedRe(), (_, id, arg, query) => {
        const text = String(arg).trim()
        const flags = scriptFlags(query)
        const label = flags.hide ? scriptLabel(text, flags) : escapeMdInline(text)
        return `[${label}](${scriptHref(id, text, flags)})`
      })
    })
    .join('')
  return withMdLinks
}

/** Rendered notes by source and the labels their refs resolve to: switching tabs back to a note must not redo marked + DOMPurify. */
const RENDER_CACHE_MAX = 60
const renderCache = new Map<string, string>()

function renderKey(text: string, labels: Record<string, string>, values: Record<string, string>) {
  const refs = new Set<string>()
  for (const m of text.matchAll(REF_RE)) refs.add(`${m[1]}:${m[2]}`)
  let sig = ''
  for (const r of refs) sig += `\u0001${labels[r] ?? ''}`
  const names = Object.keys(values).sort()
  for (const name of names) sig += `\u0002${name}=${values[name]}`
  return `${text}\u0000${sig}`
}

/** Render markdown to sanitized HTML. Journal refs become in-app `?kind=id` links. `{{name}}` becomes a value. */
export function renderMarkdown(
  source: any,
  opts: { labels?: Record<string, string>; values?: Record<string, string> } = {}
) {
  const text = String(source ?? '')
  if (!text.trim()) return ''
  const values = opts.values ?? {}
  const key = renderKey(text, opts.labels ?? {}, values)
  const hit = renderCache.get(key)
  if (hit !== undefined) {
    renderCache.delete(key)
    renderCache.set(key, hit)
    return hit
  }
  const bag: string[] = []
  let dirty = marked.parse(linkifyRefs(text, opts, bag), { async: false })
  if (typeof dirty !== 'string' || typeof window === 'undefined') return ''
  ensureLinkHook()
  dirty = shapeTables(dirty)
  let html = DOMPurify.sanitize(dirty, PURIFY_OPTS)
  if (bag.length) html = html.replace(/⟦V(\d+)⟧/g, (_, n) => bag[Number(n)] ?? '')
  renderCache.set(key, html)
  if (renderCache.size > RENDER_CACHE_MAX) renderCache.delete(renderCache.keys().next().value as string)
  return html
}
