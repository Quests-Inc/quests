/** One field for a named value: a constant is a formula of one term, so the text decides which it is. */

export type ValueInput = { value: number | string | boolean; formula: string } | { error: string }

const STR_OPERATORS = /\+|==|!=|<=|>=|<|>/

function quoted(raw: string) {
  return raw.length >= 2 && raw.startsWith('"') && raw.endsWith('"')
}

/**
 * `str`: outer quotes make a value whatever is inside; else an operator (`+`, comparisons) makes a formula; else a value.
 * `int`, `float`: a number is a value, anything else a formula. `bool`: да/нет/true/false is a value, else a formula.
 */
export function readValueInput(type: string, raw: string): ValueInput {
  if (type === 'str') {
    if (quoted(raw)) return { value: raw.slice(1, -1), formula: '' }
    if (STR_OPERATORS.test(raw)) return { value: '', formula: raw.trim() }
    return { value: raw, formula: '' }
  }
  const text = raw.trim()
  if (!text) return { error: 'нужно значение' }
  if (type === 'bool') {
    const word = text.toLowerCase()
    if (word === 'true' || word === 'да') return { value: true, formula: '' }
    if (word === 'false' || word === 'нет') return { value: false, formula: '' }
    return { value: '', formula: text }
  }
  const n = Number(text)
  if (Number.isFinite(n) && /^[+-]?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i.test(text)) {
    return { value: type === 'int' ? Math.trunc(n) : n, formula: '' }
  }
  return { value: '', formula: text }
}

/** The text a stored row shows in the field, so reading it back gives the same row. */
export function valueInputText(row: { type: string; value: unknown; formula?: string }): string {
  if (row.formula) return row.formula
  if (row.value == null) return ''
  if (row.type === 'bool') return row.value === true || row.value === 'true' ? 'да' : 'нет'
  const text = String(row.value)
  if (row.type === 'str' && (quoted(text) || STR_OPERATORS.test(text))) return `"${text}"`
  return text
}
