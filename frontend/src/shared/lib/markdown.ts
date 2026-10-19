import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import css from 'highlight.js/lib/languages/css'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import go from 'highlight.js/lib/languages/go'
import ini from 'highlight.js/lib/languages/ini'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdownLang from 'highlight.js/lib/languages/markdown'
import python from 'highlight.js/lib/languages/python'
import rust from 'highlight.js/lib/languages/rust'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'
import { marked } from 'marked'

import { REF_RE, refHref } from './refs.ts'

hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', bash)
hljs.registerLanguage('sh', bash)
hljs.registerLanguage('zsh', bash)
hljs.registerLanguage('css', css)
hljs.registerLanguage('dockerfile', dockerfile)
hljs.registerLanguage('docker', dockerfile)
hljs.registerLanguage('go', go)
hljs.registerLanguage('golang', go)
hljs.registerLanguage('ini', ini)
hljs.registerLanguage('toml', ini)
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('json', json)
hljs.registerLanguage('markdown', markdownLang)
hljs.registerLanguage('md', markdownLang)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('rust', rust)
hljs.registerLanguage('rs', rust)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('svg', xml)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)

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
  const src = String(code ?? '').replace(/\n$/, '')
  if (lang && hljs.getLanguage(lang)) {
    try {
      return hljs.highlight(src, { language: lang, ignoreIllegals: true }).value
    } catch {
      /* fall through */
    }
  }
  return escapeHtml(src)
}

function escapeHtml(s: string) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
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
  ALLOWED_ATTR: ['href', 'rel', 'target', 'src', 'alt', 'align', 'class'],
  ADD_ATTR: ['data-lang'],
  ALLOW_DATA_ATTR: false,
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

const SCRIPT_EMBED_RE = /\bscript=(\d+)\{([^{}[\]\n]{1,200})\}/g

function splitCode(src: string) {
  return String(src || '').split(/(```[\s\S]*?```|`[^`]+`)/)
}

function linkifyRefs(source: string, opts: { labels?: Record<string, string> } = {}) {
  const labels = opts.labels || {}
  const withMdLinks = splitCode(source)
    .map((part, i) => {
      if (i % 2 === 1) return part
      const chunk = part
        .replace(ATTACHMENT_IMG_RE, (_, alt, id) => `![${alt}](${attachmentImageSrc(id)})`)
        // `script=12{3к8}`: a live computation (shared/lib/scriptEmbed.ts); the text in braces is the script's argument.
        .replace(SCRIPT_EMBED_RE, (_, id, arg) => `[${arg.trim()}](?script=${id}&_=${encodeURIComponent(arg.trim())})`)
        .replace(REF_RE, (full, kind, id) => {
          const label = labels[`${kind}:${id}`] || full
          return `[${label}](${refHref(kind, id)})`
        })
      return chunk
    })
    .join('')
  return withMdLinks
}

/** Rendered notes by source and the labels their refs resolve to: switching tabs back to a note must not redo marked + DOMPurify. */
const RENDER_CACHE_MAX = 60
const renderCache = new Map<string, string>()

function renderKey(text: string, labels: Record<string, string>) {
  const refs = new Set<string>()
  for (const m of text.matchAll(REF_RE)) refs.add(`${m[1]}:${m[2]}`)
  let sig = ''
  for (const r of refs) sig += `\u0001${labels[r] ?? ''}`
  return `${text}\u0000${sig}`
}

/** Render markdown to sanitized HTML. Journal refs become in-app `?kind=id` links. */
export function renderMarkdown(source: any, opts: { labels?: Record<string, string> } = {}) {
  const text = String(source ?? '')
  if (!text.trim()) return ''
  const key = renderKey(text, opts.labels ?? {})
  const hit = renderCache.get(key)
  if (hit !== undefined) {
    renderCache.delete(key)
    renderCache.set(key, hit)
    return hit
  }
  const dirty = marked.parse(linkifyRefs(text, opts), { async: false })
  if (typeof dirty !== 'string' || typeof window === 'undefined') return ''
  ensureLinkHook()
  const html = DOMPurify.sanitize(dirty, PURIFY_OPTS)
  renderCache.set(key, html)
  if (renderCache.size > RENDER_CACHE_MAX) renderCache.delete(renderCache.keys().next().value as string)
  return html
}
