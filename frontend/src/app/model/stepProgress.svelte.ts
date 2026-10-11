// Прогресс шага: правка числа «выполнено из N» (клик, Enter, Esc, потеря фокуса) и кнопки +1 / −1.

type Step = { id: number; title?: string; progress_current?: number; progress_total?: number }
type Quest = { id: number; steps?: Step[] }

/** Квест, которому принадлежит шаг. */
export function questIdOfStep(quests: Quest[], stepId: number): number | null {
  for (const q of quests) if (q.steps?.some((s) => s.id === stepId)) return q.id
  return null
}

/** Шаг и его квест. */
export function stepRef(quests: Quest[], stepId: number): { step: Step; questId: number } | null {
  for (const q of quests) {
    const step = q.steps?.find((s) => s.id === stepId)
    if (step) return { step, questId: q.id }
  }
  return null
}

/** Введённое значение, приведённое к целому в пределах 0…total. */
export function clampProgress(value: unknown, total: unknown): number {
  const max = Math.max(0, Number(total) || 0)
  return Math.max(0, Math.min(max, Math.round(Number(value))))
}

export function createStepProgress(deps: {
  quests: () => Quest[]
  selectedQuestId: () => number | null
  updateQuestStep: (questId: number, stepId: number, patch: { progress_current: number }) => Promise<any>
  applyQuest: (quest: any, opts?: { select?: boolean }) => void
  setError: (message: string) => void
}) {
  let busyId = $state<number | null>(null)
  let editId = $state<number | null>(null)
  let editQuestId = $state<number | null>(null)
  let editValue = $state('')

  function closeEdit() {
    editId = null
    editQuestId = null
  }

  async function set(step: Step, value: string | number, questId: number | null = null) {
    const qid = questId ?? deps.selectedQuestId()
    if (qid == null || busyId != null) return
    const next = clampProgress(value, step.progress_total)
    if (!Number.isFinite(next) || next === step.progress_current) {
      closeEdit()
      return
    }
    busyId = step.id
    deps.setError('')
    try {
      deps.applyQuest(await deps.updateQuestStep(qid, step.id, { progress_current: next }), { select: false })
    } catch (e) {
      deps.setError((e as Error)?.message || String(e))
    } finally {
      busyId = null
      closeEdit()
    }
  }

  return {
    get busyId() {
      return busyId
    },
    get editId() {
      return editId
    },
    get editValue() {
      return editValue
    },
    set editValue(v: string) {
      editValue = v
    },
    set,
    bump: (step: Step, delta: number, questId: number | null = null) =>
      set(step, (step.progress_current ?? 0) + delta, questId),
    begin(step: Step, questId: number | null = null) {
      if (busyId != null) return
      editId = step.id
      editQuestId = questId ?? questIdOfStep(deps.quests(), step.id) ?? deps.selectedQuestId()
      editValue = String(step.progress_current)
    },
    cancel: closeEdit,
    onKeydown(event: { key: string; preventDefault: () => void }, step: Step) {
      if (event.key === 'Enter') {
        event.preventDefault()
        void set(step, editValue, editQuestId)
      } else if (event.key === 'Escape') {
        event.preventDefault()
        closeEdit()
      }
    },
    onBlur(step: { id: number } & Step) {
      if (editId !== step.id) return
      void set(step, editValue, editQuestId)
    },
  }
}
