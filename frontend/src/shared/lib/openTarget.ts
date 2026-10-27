// Переходы между вкладками из глубины интерфейса: редактор скрипта открывает запуски, хук — свой скрипт, шаг — последний
// запуск. Приложение регистрирует обработчик один раз; страницы читают параметры из адреса, когда становятся активными.

export type OpenTarget =
  /** The run log, filtered; `run` opens one run. */
  | { kind: 'runs'; run?: number; script?: number; source?: { kind: string; id: number }; state?: string }
  | { kind: 'script'; id: number }
  | { kind: 'hook'; id: number }
  /** An entity by its link kind (quest, step, note, template, questline). */
  | { kind: 'ref'; ref: string; id: number }

let handler: ((t: OpenTarget) => void) | null = null

export function setOpenTarget(h: ((t: OpenTarget) => void) | null) {
  handler = h
}

export function openTarget(t: OpenTarget) {
  handler?.(t)
}

/** The query a target leaves in the address bar for the page it opens. */
export function searchOfTarget(t: OpenTarget): Record<string, unknown> | null {
  if (t.kind === 'runs')
    return {
      tab: 'runs',
      run: t.run,
      script: t.script,
      src: t.source ? `${t.source.kind}:${t.source.id}` : undefined,
      state: t.state,
    }
  if (t.kind === 'script') return { tab: 'scripts', script: t.id }
  if (t.kind === 'hook') return { tab: 'hooks', hook: t.id }
  return null
}
