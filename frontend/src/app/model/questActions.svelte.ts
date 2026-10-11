// Быстрые действия над квестом из меню и шапки: статус, отложить, значимость, выполнено, выгрузка.
// Все, кроме выгрузки, идут по одному за раз (пока одно не закончилось, остальные игнорируются).
import { downloadQuestMarkdown, downloadQuestPdf, downloadQuestPdfSimple } from '@/entities/quest'
import { QUEST_SIGNIFICANCES } from '@/shared/api/questConsts.ts'
import { localTimeZone } from '@/shared/lib/time.ts'
import { statusToastLabel, toast, toastDone, toastProgress } from '@/shared/lib/toasts.svelte.ts'

type Quest = { id: number; status?: string; significance?: string }
type QuestFull = Quest & Record<string, unknown>

/** Что отправить, чтобы отложить квест: снова активен, срок через `minutes`, окно такой же длины (не короче минуты). */
export function postponePatch(minutes: number, now = Date.now()) {
  const secs = Math.max(60, Math.round(Number(minutes) * 60))
  return { status: 'active', deadline_at: new Date(now + secs * 1000).toISOString(), duration_seconds: secs }
}

export function createQuestActions(deps: {
  updateQuest: (id: number, patch: Record<string, unknown>) => Promise<QuestFull>
  /** Положить ответ сервера в список; `select: false` — не переключать выбранный квест. */
  applyQuest: (quest: QuestFull, opts?: { select?: boolean }) => void
  setError: (message: string) => void
  refLabels: () => Record<string, string>
}) {
  let busy = $state(false)
  let pdfBusy = false

  async function guarded(task: () => Promise<void>) {
    if (busy) return
    busy = true
    deps.setError('')
    try {
      await task()
    } catch (e) {
      const message = (e as Error)?.message || String(e)
      deps.setError(message)
      toast(message, { kind: 'error' })
    } finally {
      busy = false
    }
  }

  return {
    get busy() {
      return busy
    },
    async patchStatus(quest: Quest, status: string) {
      if (!quest) return
      await guarded(async () => {
        deps.applyQuest(await deps.updateQuest(quest.id, { status }))
        toast(`Статус: ${statusToastLabel(status)}`, { kind: 'success' })
      })
    },
    async postpone(quest: Quest, minutes: number) {
      if (!quest) return
      await guarded(async () => {
        deps.applyQuest(await deps.updateQuest(quest.id, postponePatch(minutes)))
        toast(`Отложен на ${minutes} мин`, { kind: 'success' })
      })
    },
    async patchSignificance(quest: Quest, significance: string) {
      if (!quest || significance === (quest.significance || 'common')) return
      await guarded(async () => {
        deps.applyQuest(await deps.updateQuest(quest.id, { significance }))
        const label = QUEST_SIGNIFICANCES.find((s) => s.id === significance)?.label || significance
        toast(`Значимость: ${label}`, { kind: 'success' })
      })
    },
    async toggleCompleted(quest: Quest | null) {
      if (!quest) return
      await guarded(async () => {
        const next = quest.status === 'completed' ? 'active' : 'completed'
        deps.applyQuest(await deps.updateQuest(quest.id, { status: next }), { select: false })
        toast(next === 'completed' ? 'Отмечен выполненным' : 'Снова активен', { kind: 'success' })
      })
    },
    /** mode: 'md' | 'rich' | 'simple' */
    async exportQuest(quest: QuestFull, mode: string) {
      if (!quest || pdfBusy) return
      if (mode === 'md') {
        downloadQuestMarkdown(quest)
        toast('Markdown сохранён', { kind: 'success' })
        return
      }
      pdfBusy = true
      const tid = 'pdf-quest'
      toastProgress(tid, 'Генерация PDF…')
      try {
        if (mode === 'simple') await downloadQuestPdfSimple(quest)
        else await downloadQuestPdf(quest, { labels: deps.refLabels(), tzLabel: localTimeZone() })
        toastDone(tid, 'PDF сохранён')
      } catch (e) {
        console.error(e)
        toastDone(tid, (e as Error)?.message || 'Не удалось сохранить PDF', 'error')
      } finally {
        pdfBusy = false
      }
    },
  }
}
