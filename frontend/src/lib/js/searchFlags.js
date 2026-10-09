/** `flag:name` / `-flag:name` (and `key:value`) tokens in a search box: filters typed next to the words.
 *  parseFlagQuery('деплой flag:outdated -flag:private sig:epic', FLAGS, { sig: { test(item, v) {…} } })
 *    → { text: 'деплой', include: ['outdated'], exclude: ['private'], values: [{key:'sig', value:'epic', neg:false}], unknown: [] }
 *  A flag set is { name: { label, test(item) → boolean, aliases?: string[] } } — each widget brings its own.
 *  `keys` are value filters ({ test(item, value) → boolean, label }); a token with another key stays in the text. */

const TOKEN = /(^|\s)(-?)([\p{L}]+):([^\s]*)/giu

function resolve(flags, name) {
  const key = name.toLowerCase()
  for (const [id, def] of Object.entries(flags)) {
    if (id === key || def.aliases?.includes(key)) return id
  }
  return null
}

export function parseFlagQuery(raw, flags, keys = {}) {
  const include = []
  const exclude = []
  const unknown = []
  const values = []
  const text = String(raw ?? '')
    .replace(TOKEN, (m, lead, neg, key, name) => {
      const k = key.toLowerCase()
      if (k === 'flag' || k === 'f') {
        const id = name ? resolve(flags, name) : null
        if (id) (neg ? exclude : include).push(id)
        else if (name) unknown.push(name)
        return lead
      }
      if (keys[k] && name) {
        values.push({ key: k, value: name, neg: !!neg })
        return lead
      }
      return m
    })
    .replace(/\s+/g, ' ')
    .trim()
  return { text, include, exclude, values, unknown, active: include.length + exclude.length + values.length > 0 }
}

/** Does the item pass every filter of the parsed query? */
export function passesFlags(parsed, flags, item, keys = {}) {
  if (!parsed.active) return true
  for (const id of parsed.include) if (!flags[id].test(item)) return false
  for (const id of parsed.exclude) if (flags[id].test(item)) return false
  for (const v of parsed.values) if (keys[v.key].test(item, v.value) === v.neg) return false
  return true
}

/** The flag names to offer while the caret sits in an unfinished `flag:…` token (null = nothing to offer). */
export function flagSuggestions(raw, flags) {
  const m = /(^|\s)(-?)(?:flag|f):([^\s]*)$/iu.exec(String(raw ?? ''))
  if (!m) return null
  const part = m[3].toLowerCase()
  const out = Object.entries(flags)
    .filter(([id, d]) => !part || id.startsWith(part) || d.aliases?.some((a) => a.startsWith(part)))
    .map(([id, d]) => ({ id, label: d.label }))
  return out.length ? { neg: m[2] === '-', part: m[3], items: out } : null
}

/** The query with its unfinished flag token completed to `name`. */
export function completeFlag(raw, name, neg) {
  return String(raw ?? '').replace(/(^|\s)(-?)(?:flag|f):([^\s]*)$/iu, `$1${neg ? '-' : ''}flag:${name} `)
}
