// Расписание шаблона: разбор часов и дней недели, подпись дней, лимиты эмита. Чистые функции.

import { WEEKDAY_LABELS } from '@/shared/api/templateConsts.ts'

/** Будни по умолчанию: пн–пт (0 = понедельник). */
export const WORKDAYS = [0, 1, 2, 3, 4]

export function parseLimits(text: string) {
  try {
    const v = JSON.parse(text)
    if (v && typeof v === 'object' && !Array.isArray(v)) return v
  } catch {
    /* falls through to the error below */
  }
  throw new Error('Лимиты: нужен JSON-объект, например {"max_steps": 50}')
}

export function daysLabel(set: Set<number>) {
  const ids = [...set].sort((a, b) => a - b)
  if (ids.length === 7) return 'каждый день'
  if (ids.join() === WORKDAYS.join()) return 'по будням'
  if (ids.join() === '5,6') return 'по выходным'
  return ids.map((i) => WEEKDAY_LABELS.find((d) => d.id === i)?.label.toLowerCase()).join(', ')
}

export function parseClock(raw: any, fallbackH: string, fallbackM: string) {
  const text = String(raw || '').trim()
  if (text && text.includes(':')) {
    const [hh = fallbackH, mm = fallbackM] = text.split(':')
    return {
      hour: String(Math.min(23, Math.max(0, Number(hh) || 0))).padStart(2, '0'),
      minute: String(Math.min(59, Math.max(0, Number(mm) || 0))).padStart(2, '0'),
    }
  }
  return { hour: fallbackH, minute: fallbackM }
}

export function parseWeekdays(raw: any) {
  const set = new Set<number>()
  for (const part of String(raw || '').split(',')) {
    if (!part.trim()) continue
    const n = Number(part.trim())
    if (Number.isInteger(n) && n >= 0 && n <= 6) set.add(n)
  }
  return set.size ? set : new Set(WORKDAYS)
}
