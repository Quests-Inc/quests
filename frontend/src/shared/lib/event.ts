/** Built-in marks an event may wear. Drawn in the icon stroke so the chosen colour tints them. */
export const EVENT_ICONS = ['calendar', 'flag', 'target', 'scroll', 'map', 'layers', 'document']

export const EVENT_COLOR = '#5a8a9a'

/** Phrase for a quest deadline taken from an event. */

export function offsetSeconds(days: any, hours: any, minutes: any) {
  const d = Number(days)
  const h = Number(hours)
  const m = Number(minutes)
  const dd = Number.isFinite(d) ? Math.max(0, d) : 0
  const hh = Number.isFinite(h) ? Math.max(0, h) : 0
  const mm = Number.isFinite(m) ? Math.max(0, m) : 0
  return Math.round(dd * 86400 + hh * 3600 + mm * 60)
}

export function secondsToOffset(seconds: number | null | undefined) {
  const s = Math.max(0, Number(seconds) || 0)
  const days = Math.floor(s / 86400)
  const hours = Math.floor((s % 86400) / 3600)
  const minutes = Math.floor((s % 3600) / 60)
  return {
    days: days ? String(days) : '',
    hours: hours ? String(hours) : '',
    minutes: minutes ? String(minutes) : '',
  }
}

export function offsetLabel(seconds: number) {
  const { days, hours, minutes } = secondsToOffset(seconds)
  const parts = []
  if (days) parts.push(`${days} дн`)
  if (hours) parts.push(`${hours} ч`)
  if (minutes) parts.push(`${minutes} мин`)
  return parts.join(' ')
}

export function eventPhrase(
  q: {
    event_id?: number | null
    event_edge?: string
    event_dir?: string
    event_offset_seconds?: number | null
    event_title?: string | null
  },
  opts: { asRef?: boolean } = {}
) {
  if (!q?.event_id) return ''
  // asRef: token for RefText (title resolves from the catalog). Plain text keeps «title».
  const title = opts.asRef ? `event=${q.event_id}` : q.event_title ? `«${q.event_title}»` : 'события'
  const n = Number(q.event_offset_seconds) || 0
  if (n <= 0) return q.event_edge === 'end' ? `в конце ${title}` : `в начале ${title}`
  const head = q.event_dir === 'after' ? 'через' : 'за'
  const prep = q.event_dir === 'after' ? 'после' : 'до'
  const edge = q.event_edge === 'end' ? 'конца' : 'начала'
  return `${head} ${offsetLabel(n)} ${prep} ${edge} ${title}`
}
