// Выгрузка открытого квеста в PDF: меню выбора вида («простой» / «с оформлением») и прогресс в тостах.
type Quest = Record<string, unknown>

export function createQuestPdf(deps: {
  simple: (q: Quest) => Promise<unknown>
  rich: (q: Quest) => Promise<unknown>
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
    deps.progress(tid, 'Генерация PDF…')
    try {
      if (mode === 'simple') await deps.simple(q)
      else await deps.rich(q)
      deps.done(tid, 'PDF сохранён')
    } catch (e) {
      console.error(e)
      deps.done(tid, (e as { message?: string })?.message || 'Не удалось сохранить PDF', 'error')
    } finally {
      busy = false
    }
  }

  return {
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
