// Быстрые действия над шаблоном из списка и контекстного меню: включить/выключить, дублировать, эмитировать, скопировать ссылку.
const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

type T = { id: number; enabled?: boolean; title?: string }

export function createTemplateActions(deps: {
  api: {
    update: (id: number, patch: Record<string, unknown>) => Promise<unknown>
    copy: (id: number) => Promise<unknown>
    emit: (id: number) => Promise<{ title?: string }>
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
      deps.onChanged()
      deps.notify(`Эмит: «${q.title || t.title}»`, 'success')
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

  return { toggleEnabled, duplicate, emit, copyRef }
}
