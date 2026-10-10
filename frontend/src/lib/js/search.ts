/** Client-side quest search: layout remap + light fuzzy from 6 chars. */

/** QWERTY key → same physical key on ЙЦУКЕН (ghbdtn → привет). */
const EN_TO_RU = {
  q: 'й',
  w: 'ц',
  e: 'у',
  r: 'к',
  t: 'е',
  y: 'н',
  u: 'г',
  i: 'ш',
  o: 'щ',
  p: 'з',
  '[': 'х',
  ']': 'ъ',
  a: 'ф',
  s: 'ы',
  d: 'в',
  f: 'а',
  g: 'п',
  h: 'р',
  j: 'о',
  k: 'л',
  l: 'д',
  ';': 'ж',
  "'": 'э',
  z: 'я',
  x: 'ч',
  c: 'с',
  v: 'м',
  b: 'и',
  n: 'т',
  m: 'ь',
  ',': 'б',
  '.': 'ю',
  '`': 'ё',
}

const RU_TO_EN = Object.fromEntries(Object.entries(EN_TO_RU).map(([en, ru]) => [ru, en]))

/** Fuzzy kicks in only for longer queries; span must stay close to the needle. */
const FUZZY_MIN_LEN = 6
const FUZZY_SPAN_FACTOR = 1.35
const FUZZY_SPAN_PAD = 2

function mapChars(text, table) {
  let out = ''
  for (const ch of text) {
    const lower = ch.toLowerCase()
    const mapped = table[lower]
    if (!mapped) {
      out += ch
      continue
    }
    out += ch === lower ? mapped : mapped.toUpperCase()
  }
  return out
}

/** Query variants: as typed + both layout remaps. */
export function queryVariants(raw) {
  const q = String(raw || '').trim()
  if (!q) return []
  const variants = new Set([q, mapChars(q, EN_TO_RU), mapChars(q, RU_TO_EN)])
  return [...variants].filter(Boolean)
}

function normalize(text) {
  return String(text || '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim()
}

/** Substring always; longer queries also get a tight subsequence fuzzy. */
export function textMatches(haystack, needle) {
  const h = normalize(haystack)
  const n = normalize(needle)
  if (!n) return true
  if (!h) return false
  if (h.includes(n)) return true
  if (n.length < FUZZY_MIN_LEN) return false

  const maxSpan = Math.ceil(n.length * FUZZY_SPAN_FACTOR) + FUZZY_SPAN_PAD
  let i = 0
  let start = -1
  for (let j = 0; j < h.length; j += 1) {
    if (h[j] !== n[i]) continue
    if (i === 0) start = j
    i += 1
    if (i < n.length) continue
    if (j - start + 1 <= maxSpan) return true
    // Match was too sparse — restart after the first hit.
    j = start
    i = 0
    start = -1
  }
  return false
}

export function questSearchBlob(quest) {
  const steps = (quest.steps || []).map((s) => `${s.title || ''} ${s.description || ''}`).join(' ')
  return [
    quest.title,
    quest.description,
    quest.status,
    quest.progress_label,
    quest.category_label,
    quest.category_slug,
    quest.questline_title,
    steps,
  ]
    .filter(Boolean)
    .join(' ')
}

export function questMatchesQuery(quest, rawQuery) {
  const variants = queryVariants(rawQuery)
  if (!variants.length) return true
  const blob = questSearchBlob(quest)
  return variants.some((v) => textMatches(blob, v))
}
