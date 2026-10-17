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
  ackFailure?: (id: number) => Promise<QuestFull>
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

  /** Одно и то же действие над группой: один тост на всех, ошибки по одному не прерывают остальных. */
  async function many(
    quests: Quest[],
    patchOf: (q: Quest) => Record<string, unknown> | null,
    text: (n: number) => string
  ) {
    await guarded(async () => {
      let done = 0
      let failed = 0
      for (const q of quests) {
        const patch = patchOf(q)
        if (!patch) continue
        try {
          deps.applyQuest(await deps.updateQuest(q.id, patch), { select: false })
          done += 1
        } catch {
          failed += 1
        }
      }
      if (failed) toast(`${text(done)}, не удалось: ${failed}`, { kind: 'error' })
      else if (done) toast(text(done), { kind: 'success' })
    })
  }

  return {
    get busy() {
      return busy
    },
    patchStatusMany: (quests: Quest[], status: string) =>
      many(
        quests,
        (q) => (q.status === status ? null : { status }),
        (n) => `Статус: ${statusToastLabel(status)} (${n})`
      ),
    postponeMany: (quests: Quest[], minutes: number) =>
      many(
        quests,
        () => postponePatch(minutes),
        (n) => `Отложено на ${minutes} мин (${n})`
      ),
    patchSignificanceMany: (quests: Quest[], significance: string) =>
      many(
        quests,
        (q) => (significance === (q.significance || 'common') ? null : { significance }),
        (n) => `Значимость: ${QUEST_SIGNIFICANCES.find((s) => s.id === significance)?.label || significance} (${n})`
      ),
    async patchStatus(quest: Quest, status: string) {
      if (!quest) return
      await guarded(async () => {
        deps.applyQuest(await deps.updateQuest(quest.id, { status }))
        toast(`Статус: ${statusToastLabel(status)}`, { kind: 'success' })
      })
    },
    async ackFailure(quest: Quest) {
      if (!quest || !deps.ackFailure) return
      await guarded(async () => {
        deps.applyQuest(await deps.ackFailure!(quest.id), { select: false })
        toast('Скрыт из провалов', { kind: 'success' })
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
