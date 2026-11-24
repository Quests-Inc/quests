// Выгрузка заметки: меню «Скачать», Markdown и PDF с прогрессом в тостах.
import { pdfPreferences } from '@/shared/lib/pdfSettings.svelte.ts'
import { toast, toastDone, toastProgress } from '@/shared/lib/toasts.svelte.ts'

export interface ExportPayload {
  id: number
  title: string
  description: string
}

export function createNoteExport(deps: {
  downloadMarkdown: (payload: ExportPayload) => void | Promise<void>
  downloadPdf: (payload: ExportPayload, book?: boolean, configure?: boolean) => Promise<unknown>
  /** Сообщение об ошибке для страницы ('' — очистить). */
  setError: (message: string) => void
}) {
  let menuOpen = $state(false)
  let menuX = $state(0)
  let menuY = $state(0)
  let busy = $state(false)

  async function run(payload: ExportPayload, action: string) {
    if (busy) return
    busy = true
    deps.setError('')
    try {
      if (action === 'md') {
        await deps.downloadMarkdown(payload)
        toast('Markdown сохранён', { kind: 'success' })
      } else if (action === 'pdf' || action === 'pdf-book' || action === 'pdf-settings') {
        const tid = 'pdf-note'
        if (action !== 'pdf-settings') toastProgress(tid, 'Генерация PDF…')
        try {
          const saved = await deps.downloadPdf(payload, action === 'pdf-book', action === 'pdf-settings')
          toastDone(tid, saved === false ? 'Экспорт отменён' : 'PDF сохранён')
        } catch (e) {
          toastDone(tid, (e as Error)?.message || 'Не удалось сохранить PDF', 'error')
          throw e
        }
      }
    } catch (e) {
      const message = (e as Error)?.message || String(e)
      deps.setError(message)
      // Для PDF причину уже показал тост прогресса.
      if (action !== 'pdf' && action !== 'pdf-book' && action !== 'pdf-settings') toast(message, { kind: 'error' })
    } finally {
      busy = false
    }
  }

  return {
    get menuOpen() {
      return menuOpen
    },
    set menuOpen(v: boolean) {
      menuOpen = v
    },
    get menuX() {
      return menuX
    },
    get menuY() {
      return menuY
    },
    get busy() {
      return busy
    },
    get items() {
      return [
        { id: 'md', label: 'Markdown (.md)' },
        {
          id: 'pdf-menu',
          label: busy ? 'PDF…' : `PDF · ${pdfPreferences.name}`,
          children: [
            { id: 'pdf', label: 'Только эта заметка', disabled: busy },
            { id: 'pdf-book', label: 'Сшить вложенные заметки в книгу', disabled: busy },
            { id: 'pdf-settings', label: 'Настроить экспорт…', disabled: busy },
          ],
        },
      ]
    },
    /** Меню открывается под кнопкой. */
    openMenu(event: MouseEvent) {
      event.preventDefault()
      event.stopPropagation()
      const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
      menuX = rect.left
      menuY = rect.bottom + 4
      menuOpen = true
    },
    run,
  }
}
