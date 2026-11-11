import type { RunnerJob } from '@/shared/api/execution.gen.ts'
import { parseApiDate } from '@/shared/lib/time.ts'
export function span(ms: number) {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s} с`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} мин`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h} ч ${m % 60} мин`
  return `${Math.floor(h / 24)} дн`
}

export const at = (iso: string | null | undefined) => (iso ? (parseApiDate(iso)?.getTime() ?? null) : null)

export const STATE: Record<string, { text: string; cls: string }> = {
  queued: { text: 'в очереди', cls: 'wait' },
  assigned: { text: 'взято', cls: 'run' },
  running: { text: 'идёт', cls: 'run' },
  success: { text: 'готово', cls: 'ok' },
  fail: { text: 'ошибка', cls: 'bad' },
  timeout: { text: 'таймаут', cls: 'bad' },
  lost: { text: 'связь потеряна', cls: 'warn' },
  skipped: { text: 'устройство не появилось', cls: 'warn' },
  abandoned: { text: 'результат неизвестен', cls: 'bad' },
  canceled: { text: 'отменено', cls: 'off' },
}
export const KIND: Record<string, string> = { poll: 'опрос', once: 'разово', emit_pool: 'шаблон' }

export function took(j: RunnerJob) {
  const a = at(j.started_at) ?? at(j.assigned_at)
  const b = at(j.finished_at)
  return a && b ? span(b - a) : ''
}
