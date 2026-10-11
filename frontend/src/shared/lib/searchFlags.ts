/** Filters typed into a search box next to the words: `key:value` tokens, `-key:value` for "not".
 *
 *  A widget describes its vocabulary once (a spec) and uses the same query string as the single source of
 *  truth: its selectors read their value from the query (`tokenValues`) and write back into it (`setToken`),
 *  so a selector and the typed text never disagree.
 *
 *  spec = { key: { label, values?: [{ id, label, aliases? }], test(item, valueId) → boolean, all?: boolean } }
 *   - `values` closes the vocabulary (an unknown value is reported); without it any value is accepted.
 *   - several positive tokens of one key are alternatives (status:done status:failed), unless `all: true`
 *     (flag:pinned flag:hidden need both). Negative tokens each exclude.
 *   - a token whose key is not in the spec stays in the text (it may just be a word, "http:foo").
 */

const TOKEN = /(^|\s)(-?)([\p{L}]+):([^\s]*)/giu

export type FlagValue = { id: string; label: string; aliases?: string[] }
/** One filter flag (`status:done`): which values it takes and how an item is tested against one. */
export type FlagDef = {
  label: string
  values?: FlagValue[]
  /** every value must match (default: any of them) */
  all?: boolean
  hint?: string
  test: (item: any, value: string) => boolean
}
export type FlagSpec = Record<string, FlagDef>
export type ParsedQuery = {
  text: string
  terms: { key: string; value: string; neg: boolean }[]
  unknown: string[]
  active: boolean
}

function resolve(def: FlagDef, name: string) {
  if (!def.values) return name
  const key = name.toLowerCase()
  for (const v of def.values) {
    if (v.id.toLowerCase() === key || v.aliases?.includes(key)) return v.id
  }
  return null
}

/** → { text, terms: [{key, value, neg}], unknown: ['key:value'…], active } */
export function parseQuery(raw: string, spec: FlagSpec) {
  const terms: { key: any; value: any; neg: boolean }[] = []
  const unknown: string[] = []
  const text = String(raw ?? '')
    .replace(TOKEN, (m, lead, neg, key, name) => {
      const k = key.toLowerCase()
      const def = spec[k]
      if (!def) return m
      if (!name) return m // `status:` being typed: keep it, it is not a filter yet
      const id = resolve(def, name)
      if (id == null) unknown.push(`${k}:${name}`)
      else terms.push({ key: k, value: id, neg: !!neg })
      return lead
    })
    .replace(/\s+/g, ' ')
    .trim()
  return { text, terms, unknown, active: terms.length > 0 }
}

/** The positive values of one key, in the order typed. */
export function tokenValues(parsed: ParsedQuery, key: string) {
  return parsed.terms.filter((t: { key: any; neg: any }) => t.key === key && !t.neg).map((t: { value: any }) => t.value)
}

export function hasToken(parsed: ParsedQuery, key: string, value: string) {
  return parsed.terms.some((t: { key: any; neg: any; value: any }) => t.key === key && !t.neg && t.value === value)
}

/** Does the item pass every filter of the parsed query? */
export function passes(parsed: ParsedQuery, spec: FlagSpec, item: any) {
  if (!parsed.terms.length) return true
  const byKey = new Map()
  for (const t of parsed.terms) {
    const def = spec[t.key]
    if (t.neg) {
      if (def.test(item, t.value)) return false
    } else {
      if (!byKey.has(t.key)) byKey.set(t.key, [])
      byKey.get(t.key).push(t.value)
    }
  }
  for (const [key, vals] of byKey) {
    const def = spec[key]
    const ok = def.all ? vals.every((v: any) => def.test(item, v)) : vals.some((v: any) => def.test(item, v))
    if (!ok) return false
  }
  return true
}

/** Rewrite the query so that `key` has exactly these positive values (none removes the key's tokens).
 *  Negative tokens of the key are kept. Free text and other tokens are untouched. */
export function setToken(raw: string, key: string, values: string[] | null | undefined) {
  const list = (values ?? []).filter((v: string) => v !== '' && v != null)
  const kept = String(raw ?? '')
    .replace(TOKEN, (m, lead, neg, k, name) => (!neg && k.toLowerCase() === key && name ? lead : m))
    .replace(/\s+/g, ' ')
    .trim()
  const add = list.map((v: any) => `${key}:${v}`).join(' ')
  return [kept, add].filter(Boolean).join(' ') + (add ? ' ' : '')
}

/** Toggle one value of a key on/off in the query. */
export function toggleToken(raw: string, parsed: ParsedQuery, key: string, value: string) {
  const cur = tokenValues(parsed, key)
  return setToken(raw, key, cur.includes(value) ? cur.filter((v: any) => v !== value) : [...cur, value])
}

/** Values to offer while the caret sits in an unfinished `key:part` token (null = nothing to offer). */
export function suggestions(raw: string, spec: FlagSpec) {
  const m = /(^|\s)(-?)([\p{L}]+):([^\s]*)$/iu.exec(String(raw ?? ''))
  if (!m) return null
  const def = spec[m[3].toLowerCase()]
  if (!def?.values) return null
  const part = m[4].toLowerCase()
  const items = def.values.filter(
    (v) => !part || v.id.toLowerCase().startsWith(part) || v.aliases?.some((a) => a.startsWith(part))
  )
  return items.length ? { key: m[3].toLowerCase(), neg: m[2] === '-', items } : null
}

/** The query with its unfinished token completed to `value`. */
export function completeToken(raw: string, key: string, value: string, neg?: boolean) {
  return String(raw ?? '').replace(/(^|\s)(-?)([\p{L}]+):([^\s]*)$/iu, `$1${neg ? '-' : ''}${key}:${value} `)
}

/** "key:a|b" help line for a title attribute. */
export function helpLine(spec: Record<string, { values?: { id: string }[]; hint?: string }>) {
  return Object.entries(spec)
    .map(([k, d]) => `${k}:${d.values ? d.values.map((v) => v.id).join('|') : d.hint || '…'}`)
    .join('  ')
}
