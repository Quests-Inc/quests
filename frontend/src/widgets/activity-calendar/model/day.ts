import type { EventRead } from '@/shared/api/journal.gen'
import { parseApiDate } from '@/shared/lib/time.ts'

export function eventColor(color: string | undefined) {
  return color && /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(color) ? color : '#5a8a9a'
}

export function monthKey(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

export function dayKey(d: Date) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function monthStartKey(d = new Date()) {
  return dayKey(new Date(d.getFullYear(), d.getMonth(), 1))
}

export function monthEndKey(d = new Date()) {
  return dayKey(new Date(d.getFullYear(), d.getMonth() + 1, 0))
}

export function parseDayKey(key: string) {
  const [y, m, d] = key.split('-').map(Number)
  return new Date(y, m - 1, d)
}

export function dayBounds(key: string) {
  const start = parseDayKey(key)
  start.setHours(0, 0, 0, 0)
  const end = new Date(start)
  end.setDate(end.getDate() + 1)
  return { start, end }
}

export function formatTime(d: Date) {
  return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

export function formatDayTitle(key: string) {
  return parseDayKey(key).toLocaleDateString('ru-RU', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  })
}

export function sigKey(q: { significance: any }) {
  const s = String(q?.significance || 'common').toLowerCase()
  if (['insignificant', 'common', 'uncommon', 'epic', 'legendary'].includes(s)) return s
  return 'common'
}

/** Urgent window [deadline − duration, deadline]. Point if no duration. */
export function questWindow(q: any): { start: Date; end: Date } | null {
  const deadline = parseApiDate(q?.deadline_at)
  if (!deadline) return null
  const dur = Number(q.duration_seconds) || 0
  if (dur > 0) {
    return { start: new Date(deadline.getTime() - dur * 1000), end: deadline }
  }
  return { start: deadline, end: deadline }
}

/** Planned urgent window, clipped to actual finish when completed/failed. */
export function actualWindow(
  q: any
): { start: Date; end: Date; finish: Date | null; markKind: 'completed' | 'failed' | null } | null {
  const planned = questWindow(q)
  if (!planned) return null
  const status = String(q.status || '')
  let finish: Date | null = null
  let markKind: 'completed' | 'failed' | null = null
  if (status === 'completed') {
    finish = parseApiDate(q.completed_at)
    if (finish) markKind = 'completed'
  } else if (status === 'failed') {
    finish = parseApiDate(q.updated_at)
    if (finish) markKind = 'failed'
  }
  if (!finish) {
    return { start: planned.start, end: planned.end, finish: null, markKind: null }
  }
  // Fact load: from window open to finish (shorten or extend past deadline).
  let start = planned.start
  let end = finish
  if (end.getTime() < start.getTime()) {
    start = finish
    end = finish
  }
  return { start, end, finish, markKind }
}

export function clipToDay(win: { start: Date; end: Date }, dayStart: Date, dayEnd: Date) {
  const s = Math.max(win.start.getTime(), dayStart.getTime())
  const e = Math.min(win.end.getTime(), dayEnd.getTime())
  if (e < s) return null
  if (e === s) return { startMs: s, endMs: s, point: true }
  return { startMs: s, endMs: e, point: false }
}

export function eventBarCaption(ev: EventRead, win: { start: Date; end: Date }, fromPrev: boolean, endToday: boolean) {
  const title = String(ev.title || '?')
  if (ev.all_day) {
    const a = dayKey(win.start)
    const b = dayKey(new Date(win.end.getTime() - 1))
    return a === b ? `${title} · весь день` : `${title} · весь день · ${a} — ${b}`
  }
  const endLabel = formatTime(win.end)
  const span = fromPrev
    ? `${formatTime(win.start)} (${dayKey(win.start)}) → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
    : `${formatTime(win.start)} → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
  return `${title} · ${span}`
}

export function eventBarTip(ev: EventRead, win: { start: Date; end: Date }, fromPrev: boolean, endToday: boolean) {
  return eventBarCaption(ev, win, fromPrev, endToday)
}

export function buildBarCaption(
  q: any,
  win: { start: Date; end: Date; finish: Date | null },
  fromPrev: boolean,
  endToday: boolean,
  markKind: string | null
) {
  const title = String(q.title || '?')
  const endLabel = formatTime(win.end)
  const span = fromPrev
    ? `${formatTime(win.start)} (${dayKey(win.start)}) → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
    : `${formatTime(win.start)} → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
  if (markKind === 'completed') return `${title} · выполнен · ${span}`
  if (markKind === 'failed') return `${title} · провален · ${span}`
  return `${title} · ${span}`
}

export function buildBarTip(q: any, win: { start: Date; end: Date }, fromPrev: boolean, endToday: boolean) {
  const parts = [String(q.title || '?')]
  if (fromPrev) parts.push(`с ${formatTime(win.start)} (${dayKey(win.start)})`)
  else parts.push(`с ${formatTime(win.start)}`)
  parts.push(`до ${formatTime(win.end)}${endToday ? '' : ` (${dayKey(win.end)})`}`)
  return parts.join(' · ')
}
