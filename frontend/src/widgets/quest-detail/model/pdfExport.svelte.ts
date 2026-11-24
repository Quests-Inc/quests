import { pdfPreferences } from '@/shared/lib/pdfSettings.svelte.ts'
// Выгрузка открытого квеста в PDF: меню выбора вида («простой» / «с оформлением») и прогресс в тостах.
type Quest = Record<string, unknown>

export function createQuestPdf(deps: {
  simple: (q: Quest) => Promise<unknown>
  rich: (q: Quest, configure?: boolean) => Promise<unknown>
  progress: (id: string, message: string) => void
  done: (id: string, message: string, kind?: 'error') => void
}) {
  let busy = $state(false)
  let menu = $state<{ open: boolean; x: number; y: number; quest: Quest | null }>({
    open: false,
    x: 0,
    y: 0,
    quest: null,
  })

  async function exportPdf(q: Quest | null, mode = 'rich') {
    if (!q || busy) return
    busy = true
    const tid = 'pdf-quest'
    if (mode !== 'settings') deps.progress(tid, 'Генерация PDF…')
    try {
      if (mode === 'simple') await deps.simple(q)
      else {
        const saved = await deps.rich(q, mode === 'settings')
        if (saved === false) {
          deps.done(tid, 'Экспорт отменён')
          return
        }
      }
      deps.done(tid, 'PDF сохранён')
    } catch (e) {
      console.error(e)
      deps.done(tid, (e as { message?: string })?.message || 'Не удалось сохранить PDF', 'error')
    } finally {
      busy = false
    }
  }

  return {
    get items() {
      return [
        { id: 'rich', label: `PDF · ${pdfPreferences.name}` },
        { id: 'simple', label: 'PDF (просто)' },
        { id: 'settings', label: 'Настроить экспорт…' },
      ]
    },
    get busy() {
      return busy
    },
    get menu() {
      return menu
    },
    openMenu(event: { clientX: number; clientY: number }, q: Quest) {
      menu = { open: true, x: event.clientX, y: event.clientY, quest: q }
    },
    closeMenu() {
      menu = { ...menu, open: false }
    },
    select(mode: string) {
      return exportPdf(menu.quest, mode)
    },
    exportPdf,
  }
}
