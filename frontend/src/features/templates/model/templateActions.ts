import type { EmitPreview } from '@/shared/api/templates.ts'

// Быстрые действия над шаблоном из списка и контекстного меню: включить/выключить, дублировать, эмитировать, скопировать ссылку.
const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

type T = { id: number; enabled?: boolean; title?: string }

export function createTemplateActions(deps: {
  api: {
    update: (id: number, patch: Record<string, unknown>) => Promise<unknown>
    copy: (id: number) => Promise<unknown>
    emit: (id: number) => Promise<{ title?: string; emitted?: boolean; detail?: string }>
    test?: (id: number) => Promise<EmitPreview>
  }
  refresh: () => Promise<unknown>
  onChanged: () => void
  setError: (message: string) => void
  notify: (message: string, kind: 'success' | 'error', ttl?: number) => void
  copyText: (text: string) => Promise<unknown>
}) {
  async function toggleEnabled(t: T) {
    try {
      await deps.api.update(t.id, { enabled: !t.enabled })
      deps.onChanged()
      await deps.refresh()
    } catch (e) {
      deps.setError(messageOf(e))
    }
  }

  async function duplicate(t: T) {
    try {
      await deps.api.copy(t.id)
      deps.onChanged()
      await deps.refresh()
    } catch (e) {
      deps.setError(messageOf(e))
    }
  }

  async function emit(t: T) {
    deps.setError('')
    try {
      const q = await deps.api.emit(t.id)
      if (q.emitted === false) {
        // A template made by a script is queued for its device: the quest appears when the device answers (an event of its own).
        deps.notify(q.detail || 'Выпуск поставлен в очередь', 'success', 6000)
        return
      }
      deps.onChanged()
      deps.notify(`Эмит: «${q.title || t.title}»`, 'success')
    } catch (e) {
      const msg = messageOf(e)
      deps.setError(msg)
      deps.notify(msg, 'error')
    }
  }

  /** Run the template's script on its device and say what quest it would make; no quest is created. */
  async function test(t: T) {
    deps.setError('')
    deps.notify('Ждём ответ устройства (до минуты)…', 'success', 3000)
    try {
      const p = await deps.api.test?.(t.id)
      if (!p) return
      deps.notify(
        previewText(p, t.title),
        p.outcome === 'failed' || p.outcome === 'pending' ? 'error' : 'success',
        9000
      )
    } catch (e) {
      const msg = messageOf(e)
      deps.setError(msg)
      deps.notify(msg, 'error')
    }
  }

  async function copyRef(t: T) {
    try {
      await deps.copyText(`template=${t.id}`)
      deps.notify(`Скопировано template=${t.id}`, 'success', 1600)
    } catch (e) {
      deps.notify(messageOf(e), 'error')
    }
  }

  return { toggleEnabled, duplicate, emit, test, copyRef }
}

/** One sentence for what a test emit found: the quest and its first steps, nothing this time, or why it failed. */
export function previewText(p: EmitPreview, fallbackTitle?: string): string {
  switch (p.outcome) {
    case 'quest': {
      const steps = p.steps ?? []
      const shown = steps
        .slice(0, 5)
        .map((s) => s.title)
        .join('; ')
      const more = steps.length > 5 ? ` и ещё ${steps.length - 5}` : ''
      const ignored = p.ignored_fields?.length ? ` (поля проигнорированы: ${p.ignored_fields.join(', ')})` : ''
      return `Проверка: «${p.title || fallbackTitle || 'квест'}», шагов ${steps.length}${steps.length ? `: ${shown}${more}` : ''}${ignored}. Квест не создан.`
    }
    case 'miss':
      return 'Проверка: скрипт ничего не вернул, квеста сейчас нет.'
    case 'pending':
      return `Проверка: ${p.error || 'устройство не ответило'}.`
    default:
      return `Проверка не удалась: ${p.error || 'ошибка'}`
  }
}
