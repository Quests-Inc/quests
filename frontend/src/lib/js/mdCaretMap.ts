/**
 * Approximate caret mapping between rendered markdown HTML and source text.
 * Not a full source-map — local substring match with proportional fallback.
 */

/** @returns offset into root.textContent */
export function caretOffsetFromPoint(root: ParentNode, clientX: number, clientY: number): number {
  let range: Range | null = null
  if (typeof document.caretRangeFromPoint === 'function') {
    range = document.caretRangeFromPoint(clientX, clientY)
  } else if (typeof document.caretPositionFromPoint === 'function') {
    const pos = document.caretPositionFromPoint(clientX, clientY)
    if (pos?.offsetNode) {
      range = document.createRange()
      range.setStart(pos.offsetNode, pos.offset)
    }
  }
  if (!range || !root.contains(range.startContainer)) {
    return (root.textContent ?? '').length
  }
  return offsetInRoot(root, range.startContainer, range.startOffset)
}

function offsetInRoot(root: ParentNode, node: Node, offset: number) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  let count = 0
  let n: Node | null
  while ((n = walker.nextNode())) {
    if (n === node) return count + offset
    count += n.textContent?.length ?? 0
  }
  return count
}

export function rangeFromTextOffset(root: ParentNode, offset: number): Range | null {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  let left = Math.max(0, offset)
  let n: Node | null
  let last: Text | null = null
  while ((n = walker.nextNode())) {
    last = n as Text
    const len = last.textContent?.length ?? 0
    if (left <= len) {
      const range = document.createRange()
      range.setStart(last, left)
      range.collapse(true)
      return range
    }
    left -= len
  }
  if (!last) return null
  const range = document.createRange()
  range.setStart(last, last.textContent?.length ?? 0)
  range.collapse(true)
  return range
}

export function mapRenderedOffsetToSource(source: string, rendered: string, renderedOffset: number) {
  const src = source ?? ''
  const ren = rendered ?? ''
  if (!src) return 0
  if (!ren) return 0
  const pos = clamp(renderedOffset, 0, ren.length)

  for (const len of [56, 40, 28, 18, 12, 8]) {
    const hit = locateByPrefix(src, ren, pos, len)
    if (hit != null) return hit
  }

  const span = Math.min(24, ren.length)
  const midStart = clamp(pos - Math.floor(span / 2), 0, Math.max(0, ren.length - span))
  const mid = ren.slice(midStart, midStart + span).trim()
  if (mid.length >= 6) {
    const idx = src.indexOf(mid)
    if (idx !== -1) {
      const rel = pos - midStart
      return clamp(idx + rel, 0, src.length)
    }
  }

  return Math.round((pos / Math.max(ren.length, 1)) * src.length)
}

export function mapSourceOffsetToRendered(source: string, rendered: string, sourceOffset: number) {
  const src = source ?? ''
  const ren = rendered ?? ''
  if (!ren) return 0
  if (!src) return 0
  const pos = clamp(sourceOffset, 0, src.length)

  for (const len of [56, 40, 28, 18, 12, 8]) {
    const start = Math.max(0, pos - len)
    let needle = src.slice(start, pos)
    needle = softenMarkdown(needle)
    if (needle.length < 4) continue
    const idx = ren.lastIndexOf(needle)
    if (idx !== -1) return idx + needle.length
    const collapsed = collapseWs(needle)
    if (collapsed.length >= 4) {
      const ridx = collapseWs(ren).lastIndexOf(collapsed)
      if (ridx !== -1) return approxExpandCollapsed(ren, ridx + collapsed.length)
    }
  }

  return Math.round((pos / Math.max(src.length, 1)) * ren.length)
}

export function scrollRootToTextOffset(
  root: ParentNode,
  renderedOffset: number,
  opts: { block?: ScrollLogicalPosition } = {}
) {
  const range = rangeFromTextOffset(root, renderedOffset)
  if (!range) return
  const node = range.startContainer
  const el = node.nodeType === Node.ELEMENT_NODE ? (node as Element) : node.parentElement
  el?.scrollIntoView({ block: opts.block ?? 'center', inline: 'nearest' })
}

export function placeTextareaCaret(ta: HTMLTextAreaElement, offset: number) {
  const o = clamp(offset, 0, ta.value.length)
  ta.focus()
  ta.setSelectionRange(o, o)
  const before = ta.value.slice(0, o)
  const line = before.split('\n').length
  const lh = parseFloat(getComputedStyle(ta).lineHeight)
  const lineHeight = Number.isFinite(lh) && lh > 0 ? lh : 20
  const pad = ta.clientHeight * 0.35
  ta.scrollTop = Math.max(0, line * lineHeight - pad)
}

function locateByPrefix(src: string, ren: string, pos: number, len: number): number | null {
  if (pos < 3) return null
  let start = Math.max(0, pos - len)
  let needle = ren.slice(start, pos)
  // Drop leading partial word for stabler matches.
  const cut = needle.search(/\s\S/)
  if (cut > 0 && needle.length - cut >= 4) needle = needle.slice(cut + 1)
  if (needle.length < 4) return null

  let idx = src.lastIndexOf(needle)
  if (idx !== -1) return idx + needle.length

  const soft = softenMarkdown(needle)
  if (soft.length >= 4 && soft !== needle) {
    idx = src.lastIndexOf(soft)
    if (idx !== -1) return idx + soft.length
  }

  const collapsed = collapseWs(needle)
  if (collapsed.length < 4) return null
  const normSrc = collapseWs(src)
  const nidx = normSrc.lastIndexOf(collapsed)
  if (nidx === -1) return null
  return approxExpandCollapsed(src, nidx + collapsed.length)
}

/** Strip light markdown chrome so needles match rendered text. */
function softenMarkdown(s: string) {
  return s
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/\*\*|__/g, '')
    .replace(/(^|[^\w])[*_](?=[^\w]|$)/g, '$1')
    .replace(/`+/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
}

function collapseWs(s: string) {
  return s.replace(/\s+/g, ' ')
}

/** Map index in whitespace-collapsed string back to approximate raw index. */
function approxExpandCollapsed(raw: any, collapsedIndex: number) {
  let ci = 0
  for (let i = 0; i < raw.length; i++) {
    const ch = raw[i]
    if (/\s/.test(ch)) {
      if (i > 0 && /\s/.test(raw[i - 1])) continue
      if (ci === collapsedIndex) return i
      ci++
      continue
    }
    if (ci === collapsedIndex) return i
    ci++
  }
  return raw.length
}

function clamp(n: number, lo: number, hi: number) {
  return Math.max(lo, Math.min(hi, n))
}
