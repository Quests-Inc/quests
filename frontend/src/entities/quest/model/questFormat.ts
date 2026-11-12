import { QUEST_SIGNIFICANCES } from '@/shared/api/questConsts.ts'
import { formatRemaining, remainingFromDeadline, timerTone } from '@/shared/lib/time.ts'

export const OPEN_STATUSES = new Set(['active', 'expired'])

export function isQuestInactive(q: { status: string }) {
  return !OPEN_STATUSES.has(q?.status)
}

export function statusColor(status: any) {
  return `var(--color-status-${status}, var(--color-fg-muted, #9a9a9a))`
}

export function periodBadge(q: { template_id: any; period_key: string }) {
  if (!q?.template_id) return null
  const key = q.period_key || ''
  return key || 'цикл'
}

export function significanceLabel(q: { significance: string }) {
  const id = q?.significance || 'common'
  return QUEST_SIGNIFICANCES.find((s) => s.id === id)?.label || id
}

/** Fraction only when the quest is quantified (more than one step-unit). */
export function quantifiedProgress(q: { steps_total: any; progress_label: any }) {
  const total = Number(q?.steps_total)
  if (!Number.isFinite(total) || total <= 1) return null
  return q.progress_label || null
}

/** When the quest has a deadline but no urgency window, colour the remaining
 *  time against a week so far-away dates stay green and near ones go red. */
const OPEN_DEADLINE_WINDOW = 7 * 86400

export function questTimer(q: any, nowMs: number) {
  if (!q?.deadline_at) return null
  if (q.status === 'completed' || q.status === 'failed') return null
  const rem = remainingFromDeadline(q.deadline_at, nowMs)
  if (rem == null || rem <= 0) return null
  // duration_seconds === 0 is an explicit "no window" (event phrase or empty
  // hours/minutes in QuestModal). Still tone by a week so the countdown is
  // not a flat grey next to quests that do have a window.
  const windowSec = Number(q.duration_seconds) > 0 ? Number(q.duration_seconds) : OPEN_DEADLINE_WINDOW
  const tone = q.timer_tone || timerTone(rem, windowSec) || null
  const remLabel = formatRemaining(rem)
  const durLabel = Number(q.duration_seconds) > 0 ? formatRemaining(q.duration_seconds) : null
  return {
    rem,
    label: remLabel,
    tone,
    detailLabel: durLabel ? `${remLabel} / ${durLabel}` : remLabel,
  }
}
