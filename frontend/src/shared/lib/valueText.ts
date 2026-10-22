/** The words a named value uses in prose and in the summary line. */

export function proseValue(type: string, value: unknown): string {
  if (type === 'bool') return value === true || value === 1 || value === '1' || value === 'true' ? 'да' : 'нет'
  if (value == null) return ''
  return String(value)
}
