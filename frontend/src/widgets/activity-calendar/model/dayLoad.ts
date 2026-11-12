import { QUEST_STATUS_LABELS } from '@/shared/api/client.ts'
import type { EventRead } from '@/shared/api/journal.gen'
import { parseApiDate } from '@/shared/lib/time.ts'

import {
  actualWindow,
  buildBarCaption,
  buildBarTip,
  clipToDay,
  dayBounds,
  dayKey,
  eventBarCaption,
  eventBarTip,
  eventColor,
  formatDayTitle,
  formatTime,
  monthKey,
  parseDayKey,
  sigKey,
} from './day.ts'
import { type CalScale, periodLabel, weekStart } from './period.ts'

export type DayQuestCard = {
  id: number
  title: string
  status: string
  significance: string
  timeMs: number
  timeLabel: string
  done: boolean
}

export type CalCell = { key: string; day: number; items: DayQuestCard[] } | null
export type CalBlock = { key: string; label: string; cells: CalCell[] }

export function questsByDayMap(quests: any[]): Record<string, DayQuestCard[]> {
  const map: Record<string, DayQuestCard[]> = {}
  for (const q of quests) {
    const raw = q?.deadline_at || q?.created_at
    const d = parseApiDate(raw)
    if (!d || q?.id == null) continue
    const key = dayKey(d)
    const status = String(q.status || 'active')
    const entry: DayQuestCard = {
      id: Number(q.id),
      title: String(q.title || '?'),
      status,
      significance: sigKey(q),
      timeMs: d.getTime(),
      timeLabel: formatTime(d),
      done: status === 'completed',
    }
    if (!map[key]) map[key] = []
    map[key].push(entry)
  }
  for (const key of Object.keys(map)) {
    map[key].sort((a, b) => a.timeMs - b.timeMs || a.id - b.id)
  }
  return map
}

export function eventsByDayMap(events: EventRead[]): Record<string, EventRead[]> {
  const map: Record<string, EventRead[]> = {}
  for (const o of events) {
    const start = parseApiDate(o.starts_at)
    const end = parseApiDate(o.ends_at)
    if (!start || !end) continue
    const push = (d: Date) => {
      const key = dayKey(d)
      if (!map[key]) map[key] = []
      map[key].push(o)
    }
    if (end.getTime() <= start.getTime()) {
      push(start)
      continue
    }
    const last = new Date(end.getTime() - 1)
    for (
      let d = new Date(start.getFullYear(), start.getMonth(), start.getDate());
      d <= last;
      d = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1)
    ) {
      push(d)
    }
  }
  return map
}

/** @param month 0-based */
export function buildMonth(year: number, month: number, questsByDay: Record<string, DayQuestCard[]>): CalBlock {
  const first = new Date(year, month, 1)
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const startPad = (first.getDay() + 6) % 7
  const cells: CalCell[] = []
  for (let i = 0; i < startPad; i++) cells.push(null)
  for (let day = 1; day <= daysInMonth; day++) {
    const d = new Date(year, month, day)
    const key = dayKey(d)
    cells.push({ key, day, items: questsByDay[key] || [] })
  }
  while (cells.length % 7 !== 0) cells.push(null)
  return {
    key: monthKey(first),
    label: first.toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' }),
    cells,
  }
}

export function buildWeek(monday: Date, questsByDay: Record<string, DayQuestCard[]>): CalBlock {
  const cells: CalCell[] = []
  for (let i = 0; i < 7; i++) {
    const d = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + i)
    const key = dayKey(d)
    cells.push({ key, day: d.getDate(), items: questsByDay[key] || [] })
  }
  const from = dayKey(monday)
  const to = cells[6]!.key
  return { key: `w-${from}`, label: periodLabel('week', from, to), cells }
}

export function buildBlocks(
  scale: CalScale,
  from: string,
  to: string,
  questsByDay: Record<string, DayQuestCard[]>
): CalBlock[] {
  if (scale === 'week') return [buildWeek(weekStart(parseDayKey(from)), questsByDay)]
  const fromD = parseDayKey(from)
  const toD = parseDayKey(to)
  const out: CalBlock[] = []
  let y = fromD.getFullYear()
  let m = fromD.getMonth()
  const ty = toD.getFullYear()
  const tm = toD.getMonth()
  while (y < ty || (y === ty && m <= tm)) {
    out.push(buildMonth(y, m, questsByDay))
    m += 1
    if (m > 11) {
      m = 0
      y += 1
    }
  }
  return out
}

/** Day board: deadline windows, no-window creates, and the status log. */
export function buildDayLoad(selectedKey: string, quests: any[], events: EventRead[]) {
  const { start: dayStart, end: dayEnd } = dayBounds(selectedKey)
  const dayMs = dayEnd.getTime() - dayStart.getTime()

  const eventRows: any[] = []
  for (const ev of events) {
    const start = parseApiDate(ev.starts_at)
    const end = parseApiDate(ev.ends_at)
    if (!start || !end || ev.id == null) continue
    const win = { start, end: end.getTime() <= start.getTime() ? start : end }
    const clipped = clipToDay(win, dayStart, dayEnd)
    if (!clipped) continue
    const fromPrev = win.start < dayStart
    const toNext = win.end > dayEnd
    const endToday = win.end > dayStart && win.end <= dayEnd
    const left = (clipped.startMs - dayStart.getTime()) / dayMs
    const rawW = (clipped.endMs - clipped.startMs) / dayMs
    eventRows.push({
      id: Number(ev.id),
      event: ev,
      title: String(ev.title || '?'),
      color: eventColor(ev.color),
      icon: ev.icon || 'calendar',
      iconUrl: ev.icon_url || null,
      left,
      width: clipped.point ? 0 : Math.max(rawW, 0.004),
      point: clipped.point,
      fromPrev,
      toNext,
      caption: eventBarCaption(ev, win, fromPrev, endToday),
      tip: eventBarTip(ev, win, fromPrev, endToday),
    })
  }
  eventRows.sort((a, b) => a.left - b.left || b.width - a.width || a.id - b.id)

  const onBoard: Set<number> = new Set()
  const rows: any[] = []
  for (const q of quests) {
    const win = actualWindow(q)
    if (!win || q?.id == null) continue
    const clipped = clipToDay(win, dayStart, dayEnd)
    if (!clipped) continue

    const status = String(q.status || 'active')
    const fromPrev = win.start < dayStart
    const toNext = win.end >= dayEnd
    const endToday = win.end >= dayStart && win.end < dayEnd

    let markMs: number | null = null
    let markKind: 'deadline' | 'completed' | 'failed' | null = null
    if (win.finish && win.finish >= dayStart && win.finish < dayEnd) {
      markMs = win.finish.getTime()
      markKind = win.markKind
    } else if (!win.finish && endToday) {
      markMs = win.end.getTime()
      markKind = 'deadline'
    }

    const left = (clipped.startMs - dayStart.getTime()) / dayMs
    const rawW = (clipped.endMs - clipped.startMs) / dayMs
    const width = clipped.point ? 0 : Math.max(rawW, 0.004)
    const id = Number(q.id)
    onBoard.add(id)

    rows.push({
      id,
      title: String(q.title || '?'),
      status,
      significance: sigKey(q),
      left,
      width,
      startMs: clipped.startMs,
      endMs: clipped.endMs,
      marker: markMs != null ? (markMs - dayStart.getTime()) / dayMs : null,
      markKind,
      point: clipped.point,
      fromPrev,
      toNext,
      caption: buildBarCaption(q, win, fromPrev, endToday, markKind),
      tip: buildBarTip(q, win, fromPrev, endToday),
    })
  }

  rows.sort((a, b) => a.left - b.left || b.width - a.width || a.id - b.id)

  const noWindow: any[] = []
  for (const q of quests) {
    if (q?.id == null) continue
    const id = Number(q.id)
    if (onBoard.has(id)) continue
    const created = parseApiDate(q.created_at)
    if (!created || created < dayStart || created >= dayEnd) continue
    noWindow.push({
      id,
      title: String(q.title || '?'),
      status: String(q.status || 'active'),
      significance: sigKey(q),
      timeLabel: formatTime(created),
    })
  }
  noWindow.sort((a, b) => a.timeLabel.localeCompare(b.timeLabel) || a.id - b.id)

  const log: any[] = []
  for (const q of quests) {
    if (q?.id == null) continue
    const created = parseApiDate(q.created_at)
    if (created && created >= dayStart && created < dayEnd) {
      log.push({
        kind: 'issued',
        label: 'выдан',
        at: created,
        timeLabel: formatTime(created),
        quest: q,
      })
    }
    const completed = parseApiDate(q.completed_at)
    if (completed && completed >= dayStart && completed < dayEnd) {
      log.push({
        kind: 'completed',
        label: QUEST_STATUS_LABELS.completed ?? 'выполнен',
        at: completed,
        timeLabel: formatTime(completed),
        quest: q,
      })
    }
    const updated = parseApiDate(q.updated_at)
    const st = String(q.status || '')
    if (
      updated &&
      updated >= dayStart &&
      updated < dayEnd &&
      (st === 'failed' || st === 'expired' || st === 'archived')
    ) {
      const sameCreate = created && Math.abs(created.getTime() - updated.getTime()) < 2000
      if (!sameCreate) {
        log.push({
          kind: st,
          label: QUEST_STATUS_LABELS[st] ?? st,
          at: updated,
          timeLabel: formatTime(updated),
          quest: q,
        })
      }
    }
  }
  log.sort((a, b) => a.at.getTime() - b.at.getTime() || Number(a.quest.id) - Number(b.quest.id))

  return {
    key: selectedKey,
    title: formatDayTitle(selectedKey),
    rows,
    eventRows,
    noWindow,
    events: log,
  }
}
