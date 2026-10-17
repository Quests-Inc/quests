/** Local wall-clock helpers (browser timezone). API wire format: UTC ISO. */

export function localTimeZone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'local'
  } catch {
    return 'local'
  }
}

/** Parse API datetime: naive ISO is treated as UTC (historical bug guard). */
export function parseApiDate(value: string | Date | null | undefined) {
  if (value == null || value === '') return null
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value
  }
  let s = String(value).trim()
  if (!s) return null
  // The database form "2026-10-17 12:00:00" (a space, no zone) is UTC too.
  s = s.replace(/^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2})/, '$1T$2')
  // "...T12:00:00" / "...T12:00:00.123456" without zone → UTC
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(s) && !/[zZ]$/.test(s) && !/[+-]\d{2}:?\d{2}$/.test(s)) {
    s += 'Z'
  }
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? null : d
}

export function formatLocal(value: any, opts = {}) {
  const d = parseApiDate(value)
  if (!d) return value ? String(value) : '—'
  return d.toLocaleString('ru-RU', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    ...opts,
  })
}

/** datetime-local value (YYYY-MM-DDTHH:mm) in the system timezone from UTC ISO. */
export function toLocalInputValue(iso: any) {
  const d = parseApiDate(iso)
  if (!d) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/**
 * Default deadline in local wall time: today, time rounded up to the next full hour.
 * E.g. 14:05 → 15:00; 14:00 → 15:00; 23:30 → tomorrow 00:00.
 */
export function defaultLocalDeadlineParts(now = new Date()) {
  const d = new Date(now.getTime())
  d.setSeconds(0, 0)
  d.setMinutes(0, 0)
  d.setHours(d.getHours() + 1)
  const pad = (n: number) => String(n).padStart(2, '0')
  return {
    date: `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`,
    hour: pad(d.getHours()),
    minute: pad(d.getMinutes()),
  }
}

/** Parse datetime-local (system local wall time) → UTC ISO string (with Z). */
export function localInputToUtcIso(localValue: any) {
  if (!localValue) return null
  // `datetime-local` has no TZ; Date parses it as local time in the browser.
  const isoLocal = localValue.length === 16 ? `${localValue}:00` : localValue
  const d = new Date(isoLocal)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString()
}

export function formatRemaining(seconds: number) {
  if (seconds == null || Number.isNaN(Number(seconds))) return ''
  const sign = seconds < 0 ? '-' : ''
  let s = Math.abs(Math.trunc(seconds))
  const h = Math.floor(s / 3600)
  s %= 3600
  const m = Math.floor(s / 60)
  const sec = s % 60
  if (h) return `${sign}${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
  return `${sign}${m}:${String(sec).padStart(2, '0')}`
}

export function timerTone(remainingSeconds: number, durationSeconds: number) {
  if (remainingSeconds == null || !durationSeconds) return null
  const frac = remainingSeconds / Math.max(1, durationSeconds)
  if (frac > 2 / 3) return 'green'
  if (frac > 1 / 3) return 'orange'
  return 'red'
}

export function remainingFromDeadline(deadlineIso: any, now = Date.now()) {
  const d = parseApiDate(deadlineIso)
  if (!d) return null
  return Math.round((d.getTime() - now) / 1000)
}
