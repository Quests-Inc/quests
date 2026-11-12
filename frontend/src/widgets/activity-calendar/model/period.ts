import { dayKey, monthEndKey, monthStartKey, parseDayKey } from './day.ts'

export type CalScale = 'week' | 'month' | 'season' | 'year'

export const CAL_SCALES: { id: CalScale; label: string; short: string }[] = [
  { id: 'week', label: 'неделя', short: 'н' },
  { id: 'month', label: 'месяц', short: 'м' },
  { id: 'season', label: 'сезон', short: 'с' },
  { id: 'year', label: 'год', short: 'г' },
]

/** Monday of the week containing `d` (local). */
export function weekStart(d: Date) {
  const x = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  x.setDate(x.getDate() - ((x.getDay() + 6) % 7))
  return x
}

export function weekEnd(d: Date) {
  const s = weekStart(d)
  return new Date(s.getFullYear(), s.getMonth(), s.getDate() + 6)
}

/** Meteorological: 0 зима, 1 весна, 2 лето, 3 осень. */
export function seasonIndex(d: Date) {
  const m = d.getMonth()
  if (m === 11 || m <= 1) return 0
  if (m <= 4) return 1
  if (m <= 7) return 2
  return 3
}

/** Season containing `d`: зима дек–фев, весна мар–май, лето июн–авг, осень сен–ноя. */
export function seasonBounds(d: Date): { start: Date; end: Date } {
  const y = d.getFullYear()
  const m = d.getMonth()
  if (m === 11) return { start: new Date(y, 11, 1), end: new Date(y + 1, 2, 0) }
  if (m <= 1) return { start: new Date(y - 1, 11, 1), end: new Date(y, 2, 0) }
  if (m <= 4) return { start: new Date(y, 2, 1), end: new Date(y, 5, 0) }
  if (m <= 7) return { start: new Date(y, 5, 1), end: new Date(y, 8, 0) }
  return { start: new Date(y, 8, 1), end: new Date(y, 11, 0) }
}

export function periodBounds(scale: CalScale, anchor: Date): { from: string; to: string } {
  if (scale === 'week') {
    return { from: dayKey(weekStart(anchor)), to: dayKey(weekEnd(anchor)) }
  }
  if (scale === 'month') {
    return { from: monthStartKey(anchor), to: monthEndKey(anchor) }
  }
  if (scale === 'season') {
    const { start, end } = seasonBounds(anchor)
    return { from: dayKey(start), to: dayKey(end) }
  }
  const y = anchor.getFullYear()
  return { from: dayKey(new Date(y, 0, 1)), to: dayKey(new Date(y, 11, 31)) }
}

/** Shift the period that contains `anchor` by `delta` units of `scale`. */
export function shiftPeriodAnchor(scale: CalScale, anchor: Date, delta: number): Date {
  const d = new Date(anchor.getFullYear(), anchor.getMonth(), anchor.getDate())
  if (scale === 'week') {
    d.setDate(d.getDate() + 7 * delta)
    return d
  }
  if (scale === 'month') {
    d.setDate(1)
    d.setMonth(d.getMonth() + delta)
    return d
  }
  if (scale === 'season') {
    const { start } = seasonBounds(d)
    start.setMonth(start.getMonth() + 3 * delta)
    return start
  }
  d.setMonth(0, 1)
  d.setFullYear(d.getFullYear() + delta)
  return d
}

export function periodLabel(scale: CalScale, from: string, to: string): string {
  const a = parseDayKey(from)
  const b = parseDayKey(to)
  if (scale === 'week') {
    if (a.getMonth() === b.getMonth() && a.getFullYear() === b.getFullYear()) {
      const month = a.toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })
      return `${a.getDate()}–${b.getDate()} ${month}`
    }
    const left = a.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
    const right = b.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' })
    return `${left} – ${right}`
  }
  if (scale === 'month') {
    return a.toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })
  }
  if (scale === 'season') {
    const names = ['зима', 'весна', 'лето', 'осень'] as const
    const idx = seasonIndex(a)
    if (idx === 0) return `зима ${a.getFullYear()}–${a.getFullYear() + 1}`
    return `${names[idx]} ${a.getFullYear()}`
  }
  return String(a.getFullYear())
}

export function periodKey(scale: CalScale, from: string, to: string) {
  return `${scale}:${from}:${to}`
}
