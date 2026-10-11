/** Tokens agents already paste: note=12, quest=23, … */

export const REF_RE = /\b(note|quest|questline|step|attachment|template)=(\d+)\b/g

export const REF_KINDS = ['note', 'quest', 'questline', 'step', 'attachment', 'template']

export function parseRefs(text: string): { kind: string; id: number }[] {
  const seen = new Set()
  const out = []
  const src = String(text || '')
  for (const m of src.matchAll(REF_RE)) {
    const kind = m[1]
    const id = Number(m[2])
    const key = `${kind}=${id}`
    if (!id || seen.has(key)) continue
    seen.add(key)
    out.push({ kind, id })
  }
  return out
}

export function refHref(kind: string, id: number) {
  return `?${kind}=${id}`
}

function escapeHtml(s: string) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

function escapeAttr(s: string) {
  return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;')
}

/** Plain text with journal refs → HTML (escaped text + `<a class="md-ref">`). */
export function renderRefTextHtml(source: string, opts: { labels?: Record<string, string> } = {}) {
  const labels = opts.labels || {}
  const text = String(source ?? '')
  if (!text) return ''
  let out = ''
  let lastIndex = 0
  const re = new RegExp(REF_RE.source, REF_RE.flags)
  for (const m of text.matchAll(re)) {
    const idx = m.index ?? 0
    if (idx > lastIndex) out += escapeHtml(text.slice(lastIndex, idx))
    const kind = m[1]
    const id = m[2]
    const full = m[0]
    const label = labels[`${kind}:${id}`] || full
    const href = refHref(kind, Number(id))
    out += `<a href="${escapeAttr(href)}" class="md-ref">${escapeHtml(label)}</a>`
    lastIndex = idx + full.length
  }
  if (lastIndex < text.length) out += escapeHtml(text.slice(lastIndex))
  return out
}

export function parseRefHref(href: string) {
  try {
    const u = new URL(href, 'http://local')
    for (const kind of REF_KINDS) {
      const raw = u.searchParams.get(kind)
      if (!raw) continue
      const id = Number(raw)
      if (Number.isFinite(id) && id > 0) return { kind, id }
    }
  } catch {
    /* ignore */
  }
  return null
}
