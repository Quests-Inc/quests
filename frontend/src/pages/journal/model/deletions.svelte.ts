// Удаления в журнале с подтверждением: квест (с анимацией ухода), шаг и квестлайн.
// У каждого — «спросили», «занято» и результат; список после удаления перечитывается.
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Step = { questId: number; stepId: number; title: string }
const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

export function createDeletions(deps: {
  selected: () => { id: number } | null
  selectQuest: (id: number) => void
  lineId: () => number | null
  clearLine: () => void
  reload: () => Promise<unknown> | unknown
  /** Квест удалён: выбор снимается, список перечитывается. */
  onQuestDeleted: (id: number) => void
  /** Несколько квестов удалены разом: выбор снимается, список перечитывается, один тост с отменой. */
  onQuestsDeleted: (ids: number[]) => void
  setError: (message: string) => void
  api: {
    deleteQuest: (id: number) => Promise<unknown>
    deleteQuestStep: (questId: number, stepId: number) => Promise<unknown>
    deleteQuestline: (id: number) => Promise<unknown>
  }
}) {
  let questAsk = $state(false)
  let questBusy = $state(false)
  let animActive = $state(false)
  let animResolve: (() => void) | null = null

  let manyIds = $state<number[]>([])
  let manyBusy = $state(false)

  let stepOpen = $state(false)
  let stepBusy = $state(false)
  let step = $state<Step | null>(null)

  let lineAsk = $state(false)
  let lineBusy = $state(false)

  function fail(e: unknown) {
    const message = messageOf(e)
    deps.setError(message)
    toast(message, { kind: 'error' })
  }

  // --- квест ---
  function askQuest(quest: { id: number } | null = deps.selected()) {
    const q = quest ?? deps.selected()
    if (!q || questBusy) return
    if (q.id !== deps.selected()?.id) deps.selectQuest(q.id)
    questAsk = true
  }
  /** Страница сообщает, что анимация ухода карточки закончилась. */
  function animDone() {
    animResolve?.()
    animResolve = null
  }
  async function confirmQuest() {
    const q = deps.selected()
    if (!q || questBusy) return
    questBusy = true
    deps.setError('')
    try {
      animActive = true
      await new Promise<void>((resolve) => {
        animResolve = resolve
      })
      animActive = false
      await deps.api.deleteQuest(q.id)
      questAsk = false
      deps.onQuestDeleted(q.id)
    } catch (e) {
      fail(e)
    } finally {
      questBusy = false
      animActive = false
    }
  }
  function cancelQuest() {
    if (!questBusy) questAsk = false
  }

  // --- несколько квестов (выделение в списке) ---
  function askMany(ids: number[]) {
    if (ids.length && !manyBusy) manyIds = [...ids]
  }
  async function confirmMany() {
    if (!manyIds.length || manyBusy) return
    manyBusy = true
    deps.setError('')
    const done: number[] = []
    let failed = 0
    for (const id of manyIds) {
      try {
        await deps.api.deleteQuest(id)
        done.push(id)
      } catch {
        failed += 1
      }
    }
    manyIds = []
    manyBusy = false
    if (done.length) deps.onQuestsDeleted(done)
    if (failed) fail(new Error(`Не удалось удалить: ${failed}`))
  }
  function cancelMany() {
    if (!manyBusy) manyIds = []
  }

  // --- шаг ---
  function askStep(d: Step) {
    step = d
    stepOpen = true
  }
  async function confirmStep() {
    if (!step || stepBusy) return
    stepBusy = true
    deps.setError('')
    try {
      await deps.api.deleteQuestStep(step.questId, step.stepId)
      stepOpen = false
      step = null
      await deps.reload()
      toast('Шаг удалён', { kind: 'success' })
    } catch (e) {
      fail(e)
    } finally {
      stepBusy = false
    }
  }
  function cancelStep() {
    if (stepBusy) return
    stepOpen = false
    step = null
  }

  // --- квестлайн ---
  function askLine() {
    lineAsk = true
  }
  async function confirmLine() {
    const id = deps.lineId()
    if (id == null || lineBusy) return
    lineBusy = true
    deps.setError('')
    try {
      await deps.api.deleteQuestline(id)
      lineAsk = false
      deps.clearLine()
      await deps.reload()
      toast('Квестлайн удалён', { kind: 'success' })
    } catch (e) {
      fail(e)
    } finally {
      lineBusy = false
    }
  }
  function cancelLine() {
    if (!lineBusy) lineAsk = false
  }

  return {
    get questAsk() {
      return questAsk
    },
    get questBusy() {
      return questBusy
    },
    get animActive() {
      return animActive
    },
    get manyIds() {
      return manyIds
    },
    get manyBusy() {
      return manyBusy
    },
    get stepOpen() {
      return stepOpen
    },
    get stepBusy() {
      return stepBusy
    },
    get step() {
      return step
    },
    get lineAsk() {
      return lineAsk
    },
    get lineBusy() {
      return lineBusy
    },
    askQuest,
    askMany,
    confirmMany,
    cancelMany,
    animDone,
    confirmQuest,
    cancelQuest,
    askStep,
    confirmStep,
    cancelStep,
    askLine,
    confirmLine,
    cancelLine,
  }
}
