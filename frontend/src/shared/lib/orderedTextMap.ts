/** A bounded, monotonic alignment: repeated words retain their occurrence order. */
type Word = { text: string; start: number; end: number }
type Pair = { source: Word; rendered: Word }
const WINDOW = 96
const words = (text: string): Word[] =>
  [...text.matchAll(/[\p{L}\p{N}_]+/gu)].map((m) => ({ text: m[0], start: m.index, end: m.index + m[0].length }))
let cached: { source: string; rendered: string; pairs: Pair[] } | null = null

function align(source: string, rendered: string) {
  if (cached?.source === source && cached.rendered === rendered) return cached.pairs
  const a = words(source),
    b = words(rendered),
    pairs: Pair[] = []
  let i = 0,
    j = 0
  const key = (list: Word[], at: number) =>
    list
      .slice(at, at + 3)
      .map((w) => w.text)
      .join('\0')
  while (i < a.length && j < b.length) {
    if (a[i].text === b[j].text) {
      pairs.push({ source: a[i++], rendered: b[j++] })
      continue
    }
    // Find a short common phrase after markup, URLs or generated labels. The
    // lookahead is fixed, so repeated text cannot make this quadratic.
    const candidates = new Map<string, number>()
    for (let x = i; x < Math.min(a.length - 2, i + WINDOW); x++) {
      const k = key(a, x)
      if (!candidates.has(k)) candidates.set(k, x)
    }
    let next: { i: number; j: number; distance: number } | null = null
    for (let y = j; y < Math.min(b.length - 2, j + WINDOW); y++) {
      const x = candidates.get(key(b, y))
      if (x === undefined) continue
      const distance = x - i + y - j
      if (!next || distance < next.distance) next = { i: x, j: y, distance }
    }
    if (next) {
      i = next.i
      j = next.j
    } else {
      i++
      j++
    }
  }
  cached = { source, rendered, pairs }
  return pairs
}

export function orderedTextOffset(source: string, rendered: string, offset: number, reverse = false): number | null {
  const pairs = align(source, rendered)
  const from = (p: Pair) => (reverse ? p.source : p.rendered)
  const to = (p: Pair) => (reverse ? p.rendered : p.source)
  let lo = 0,
    hi = pairs.length
  while (lo < hi) {
    const mid = (lo + hi) >>> 1
    if (from(pairs[mid]).end < offset) lo = mid + 1
    else hi = mid
  }
  const next = pairs[lo],
    previous = pairs[lo - 1]
  if (next && offset >= from(next).start)
    return to(next).start + Math.min(offset - from(next).start, to(next).end - to(next).start)
  // Whitespace and punctuation between adjacent aligned words. Do not guess
  // across a large generated block: the caller retains its fallback there.
  if (next && previous && from(next).start - from(previous).end <= 64 && to(next).start - to(previous).end <= 64) {
    const ratio = (offset - from(previous).end) / Math.max(1, from(next).start - from(previous).end)
    return Math.round(to(previous).end + ratio * (to(next).start - to(previous).end))
  }
  if (next && from(next).start - offset <= 3) return to(next).start
  if (previous && offset - from(previous).end <= 3) return to(previous).end
  return null
}
