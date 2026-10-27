// Как журнал запусков называет источники, состояния и значения.
import type { RunSourceKind } from '@/shared/api/client.ts'

export const SOURCE_WORDS: Record<RunSourceKind, string> = {
  step: 'шаг',
  note: 'заметка',
  template: 'шаблон',
  hook: 'хук',
  embed: 'вставка',
  test: 'проверка',
}

export const STATE_WORDS: Record<string, string> = { done: 'готово', failed: 'ошибка', waiting: 'ждёт раннер' }

/** States of a runner job (runner_job.state). */
export const JOB_STATE_WORDS: Record<string, string> = {
  queued: 'в очереди',
  assigned: 'взято',
  running: 'идёт',
  success: 'готово',
  fail: 'ошибка',
  timeout: 'таймаут',
  lost: 'связь потеряна',
  skipped: 'устройство не появилось',
  abandoned: 'результат неизвестен',
  canceled: 'отменено',
}

export const NODE_STATE_WORDS: Record<string, string> = {
  ran: 'сработала',
  skipped: 'пропущена',
  failed: 'ошибка',
  waiting: 'ждёт раннер',
}

/** A value on a link as one line: text as it is, the rest as JSON; long ones are cut. */
export function shortValue(v: unknown, max = 160): string {
  const s = typeof v === 'string' ? v : JSON.stringify(v)
  if (s == null) return '—'
  return s.length > max ? `${s.slice(0, max)}…` : s
}

/** Whether a value deserves the expanded block (an object, a list, a long or multi-line text). */
export function isLong(v: unknown): boolean {
  if (v !== null && typeof v === 'object') return true
  return typeof v === 'string' && (v.length > 160 || v.includes('\n'))
}

/** The filter of the run log from the address bar (`run`, `script`, `src=kind:id`, `state`). */
export function filterFromUrl(href = location.href) {
  const p = new URL(href).searchParams
  const num = (k: string) => {
    const n = Number(p.get(k))
    return Number.isFinite(n) && n > 0 ? n : undefined
  }
  const [kind, id] = (p.get('src') ?? '').split(':')
  const sourceId = Number(id)
  const state = p.get('state') ?? ''
  return {
    run: num('run'),
    script: num('script'),
    source: kind && sourceId > 0 ? { kind: kind as RunSourceKind, id: sourceId } : undefined,
    state: state === 'failed' || state === 'waiting' || state === 'done' ? state : '',
  } as const
}
