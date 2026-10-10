import { QUEST_SIGNIFICANCES } from './api.js'
import { formatRemaining, remainingFromDeadline, timerTone } from './time.js'

export const OPEN_STATUSES = new Set(['active', 'expired'])

export function isQuestInactive(q) {
  return !OPEN_STATUSES.has(q?.status)
}

export function statusColor(status) {
  return `var(--color-status-${status}, var(--color-fg-muted, #9a9a9a))`
}

export function periodBadge(q) {
  if (!q?.template_id) return null
  const key = q.period_key || ''
  return key || 'цикл'
}

export function significanceLabel(q) {
  const id = q?.significance || 'common'
  return QUEST_SIGNIFICANCES.find((s) => s.id === id)?.label || id
}

/** Fraction only when the quest is quantified (more than one step-unit). */
export function quantifiedProgress(q) {
  const total = Number(q?.steps_total)
  if (!Number.isFinite(total) || total <= 1) return null
  return q.progress_label || null
}

/** @param {any} q @param {number} nowMs */
export function questTimer(q, nowMs) {
  if (!q?.deadline_at) return null
  if (q.status === 'completed' || q.status === 'failed') return null
  const rem = remainingFromDeadline(q.deadline_at, nowMs)
  if (rem == null || rem <= 0) return null
  // No duration ⇒ no window to measure urgency against — stay neutral
  // (untoned) rather than defaulting to "red", which read as false urgency.
  // duration_seconds === 0 is an explicit "no window" state (see
  // QuestModal.svelte), same as null/undefined here.
  const tone = q.timer_tone || timerTone(rem, q.duration_seconds) || null
  const remLabel = formatRemaining(rem)
  const durLabel = q.duration_seconds ? formatRemaining(q.duration_seconds) : null
  return {
    rem,
    label: remLabel,
    tone,
    detailLabel: durLabel ? `${remLabel} / ${durLabel}` : remLabel,
  }
}
