/** Hours+minutes form fields ⇄ seconds, plus short Russian labels for the
 * collapsed-section summaries. */

/** @param {number | null | undefined} seconds */
export function secondsToParts(seconds) {
  const s = Number(seconds) || 0
  if (s <= 0) return { hours: '', minutes: '' }
  return { hours: String(Math.floor(s / 3600)), minutes: String(Math.floor((s % 3600) / 60)) }
}

/** Empty/zero on both → null ("not set"). */
export function partsToSeconds(hours, minutes) {
  const h = Number(hours)
  const m = Number(minutes)
  const hh = Number.isFinite(h) ? Math.max(0, h) : 0
  const mm = Number.isFinite(m) ? Math.max(0, m) : 0
  const total = Math.round(hh * 3600 + mm * 60)
  return total > 0 ? total : null
}

/** "2 ч", "1 ч 30 мин", "45 мин"; '' when unset. */
export function durationLabel(hours, minutes) {
  const total = partsToSeconds(hours, minutes)
  if (!total) return ''
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  if (h && m) return `${h} ч ${m} мин`
  return h ? `${h} ч` : `${m} мин`
}

/** Russian noun agreement: plural(3, ['шаг', 'шага', 'шагов']) → "шага". */
export function plural(n, [one, few, many]) {
  const a = Math.abs(n) % 100
  const b = a % 10
  if (a > 10 && a < 20) return many
  if (b === 1) return one
  if (b >= 2 && b <= 4) return few
  return many
}
