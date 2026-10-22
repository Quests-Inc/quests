import { proseValue } from '@/shared/lib/valueText.ts'

/** How a named value is shown in prose and in the summary line. */

export function formatValue(type: string, value: unknown): string {
  return proseValue(type, value)
}

export function effectiveText(rows: { name: string; type: string; value: unknown }[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) out[row.name] = formatValue(row.type, row.value)
  return out
}

export function valueSummary(rows: { name: string; text: string }[], limit = 4): string {
  if (!rows.length) return ''
  const head = rows.slice(0, limit).map((row) => `${row.name} ${row.text}`)
  const more = rows.length - head.length
  return more > 0 ? `${head.join(' · ')} · ещё ${more}` : head.join(' · ')
}
