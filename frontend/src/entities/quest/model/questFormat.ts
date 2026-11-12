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

/** Both fixed and event-derived deadlines use their real urgency window. */
export function questTimer(q: any, nowMs: number) {
  if (!q?.deadline_at || (q.status && !OPEN_STATUSES.has(q.status))) return null
  const rem = remainingFromDeadline(q.deadline_at, nowMs)
  if (rem == null) return null
  if (rem <= 0) {
    const overdue = q.status === 'expired' || (q.expire_on_deadline ?? true)
    return {
      rem,
      label: overdue ? 'просрочено' : 'срок наступил',
      tone: overdue ? 'overdue' : null,
      detailLabel: overdue ? 'просрочено' : 'срок наступил',
    }
  }
  const tone = timerTone(rem, Number(q.duration_seconds))
  const label = formatRemaining(rem)
  const window = Number(q.duration_seconds) > 0 ? formatRemaining(q.duration_seconds) : null
  return { rem, label, tone, detailLabel: window ? `${label} / ${window}` : label }
}
