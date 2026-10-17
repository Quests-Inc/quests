// Действия журнала: меню, модалки квеста и квестлайна, удаление, прогресс шага, закрепление.
// Оболочка только создаёт стол и открывает его с шапки; вкладка журнала и оверлеи читают его.
import { untrack } from 'svelte'

import { replaceQuest } from '@/entities/quest'
import {
  ackQuestFailure,
  deleteQuest,
  deleteQuestline,
  deleteQuestStep,
  updateQuest,
  updateQuestStep,
} from '@/shared/api/client.ts'
import { copyText } from '@/shared/lib/clipboard.ts'
import { createMultiSelect } from '@/shared/lib/multiSelect.svelte.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'
import { toastTrashed } from '@/shared/lib/trashUndo.ts'

import { createDeletions } from './deletions.svelte.ts'
import { createJournalMenu } from './journalMenu.svelte.ts'
import { createQuestActions } from './questActions.svelte.ts'
import { createStepProgress, questIdOfStep, stepRef } from './stepProgress.svelte.ts'

type QuestRow = {
  id: number
  title?: string
  pinned?: boolean
  overlay_hidden?: boolean
  steps?: { id: number; done?: boolean; progress_current?: number; progress_total?: number }[]
}

export function createJournalDesk(deps: {
  getQuests: () => QuestRow[]
  getQuestlines: () => { id: number; title: string }[]
  getSelected: () => QuestRow | null
  getSelectedId: () => number | null
  setSelectedId: (id: number | null) => void
  selectQuest: (id: number, opts?: { pushUrl?: boolean }) => void
  reload: () => void
  setError: (message: string) => void
  getRefLabels: () => Record<string, string>
  openSecrets: (owner: { kind: string; id: number } | null) => void
  onGraphDirty: () => void
}) {
  const remoteEdits = new Map<number, number>()
  const REMOTE_EDIT_TTL_MS = 8000

  let modalOpen = $state(false)
  let modalMode = $state('create' as 'create' | 'edit')
  let modalDefaults = $state(
    null as { questline_id?: number | null; category_id?: number | null; description?: string } | null
  )
  let lineModalOpen = $state(false)
  let lineModalMode = $state('create' as 'create' | 'edit')
  let lineModalTarget = $state(null as { id: number } | null)
  let stepModalOpen = $state(false)
  let stepModalQuestId = $state(null as number | null)
  let stepModalStep = $state(null as { id: number; title?: string } | null)
  let pinBusyId = $state(null as number | null)
  let kbStepId = $state(null as number | null)

  function noteRemote(questId: number) {
    remoteEdits.set(questId, Date.now())
  }
  function consumeRemoteEdit(questId: number | null) {
    if (questId == null) return false
    const at = remoteEdits.get(questId)
    if (at == null) return false
    remoteEdits.delete(questId)
    return Date.now() - at < REMOTE_EDIT_TTL_MS
  }

  function openCreate(
    defaults: { questline_id?: number | null; category_id?: number | null; description?: string } | Event | null = null
  ) {
    modalMode = 'create'
    modalDefaults =
      defaults && !(defaults instanceof Event)
        ? {
            questline_id: defaults.questline_id ?? null,
            category_id: defaults.category_id ?? null,
            description: defaults.description ?? '',
          }
        : null
    modalOpen = true
  }
  function openCreateQuestline() {
    lineModalMode = 'create'
    lineModalTarget = null
    lineModalOpen = true
  }
  function openEditQuestline(line: number | { id: number }) {
    const row = typeof line === 'number' ? deps.getQuestlines().find((l) => l.id === line) : line
    if (!row) return
    lineModalMode = 'edit'
    lineModalTarget = row
    lineModalOpen = true
  }
  function openEdit(quest: { id?: number } | Event | null = deps.getSelected()) {
    const selected = deps.getSelected()
    const q = quest instanceof Event ? selected : quest || selected
    if (!q?.id) return
    deps.setSelectedId(q.id)
    modalMode = 'edit'
    modalDefaults = null
    modalOpen = true
  }
  function closeQuestModal() {
    modalOpen = false
    modalDefaults = null
  }
  function closeStepModal() {
    stepModalOpen = false
    stepModalStep = null
    stepModalQuestId = null
  }
  function closeLineModal() {
    lineModalOpen = false
    lineModalTarget = null
  }

  async function copyId(kind: string, id: number) {
    if (id == null) return
    try {
      const text = `${kind}=${id}`
      await copyText(text)
      toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e)
      deps.setError(message)
      toast(message, { kind: 'error' })
    }
  }

  /** Ссылки на несколько сущностей разом: по одной на строку. */
  async function copyMany(kind: string, ids: number[]) {
    if (!ids.length) return
    try {
      await copyText(ids.map((id) => `${kind}=${id}`).join('\n'))
      toast(`Скопировано ссылок: ${ids.length}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e)
      deps.setError(message)
      toast(message, { kind: 'error' })
    }
  }

  const multi = createMultiSelect({ current: () => deps.getSelectedId() })
  // Перезагрузка списка или удаление не оставляют в выделении квесты, которых уже нет.
  $effect.root(() => {
    $effect(() => {
      const ids = new Set(deps.getQuests().map((q) => q.id))
      untrack(() => multi.prune((id) => ids.has(id)))
    })
  })

  function applyQuest(quest: { id: number }, { select = true } = {}) {
    if (!replaceQuest(quest)) deps.reload()
    if (select) deps.setSelectedId(quest.id)
  }
  function onSaved(quest: { id: number }, meta: { mode?: string } = {}) {
    deps.setSelectedId(quest.id)
    deps.reload()
    if (meta.mode === 'create') toast('Квест создан', { kind: 'success' })
    deps.onGraphDirty()
  }
  function onDeleted(id: number) {
    if (deps.getSelectedId() === id) deps.setSelectedId(null)
    deps.reload()
    toastTrashed('quest', [id], 'Квест удалён')
  }
  function onQuestlineSaved(_line: unknown, meta: { mode?: string } = {}) {
    deps.reload()
    if (meta.mode === 'create') toast('Квестлайн создан', { kind: 'success' })
  }
  function onQuestlineDeleted() {
    deps.reload()
    toast('Квестлайн удалён', { kind: 'success' })
  }

  const questActions = createQuestActions({
    updateQuest,
    ackFailure: (id) => ackQuestFailure(id),
    applyQuest: (quest, opts) => applyQuest(quest, opts),
    setError: (message) => deps.setError(message),
    refLabels: () => deps.getRefLabels(),
  })

  const steps = createStepProgress({
    quests: () => deps.getQuests(),
    selectedQuestId: () => deps.getSelected()?.id ?? null,
    updateQuestStep,
    applyQuest: (quest, opts) => applyQuest(quest, opts),
    setError: (message) => deps.setError(message),
  })

  const del = createDeletions({
    selected: () => deps.getSelected(),
    selectQuest: (id) => deps.setSelectedId(id),
    lineId: () => menu.lineId,
    clearLine: () => menu.clearLine(),
    reload: () => deps.reload(),
    onQuestDeleted: (id) => onDeleted(id),
    setError: (message) => deps.setError(message),
    api: { deleteQuest, deleteQuestStep, deleteQuestline },
  })

  const menu = createJournalMenu({
    quests: () => deps.getQuests(),
    picked: (id) => multi.targets(id),
    clearPicked: () => multi.clear(),
    copyMany: (kind, ids) => void copyMany(kind, ids),
    questlines: () => deps.getQuestlines(),
    findStep: (stepId) => stepRef(deps.getQuests(), stepId),
    selectQuest: (id) => deps.setSelectedId(id),
    selectedQuestId: () => deps.getSelectedId(),
    copyId: (kind, id) => void copyId(kind, id),
    openSecrets: (owner) => deps.openSecrets(owner),
    openCreate: (defaults) => openCreate(defaults),
    editQuest: (q) => openEdit(q),
    editLine: (line) => openEditQuestline(line),
    askDeleteLine: () => del.askLine(),
    askDeleteQuest: (q) => del.askQuest(q),
    editStep: (questId, step) => {
      stepModalQuestId = questId
      stepModalStep = step
      stepModalOpen = true
    },
    askDeleteStep: (d) => del.askStep(d),
    quest: questActions,
    step: { bump: (s, d, q) => steps.bump(s, d, q), set: (s, v, q) => steps.set(s, v, q) },
  })

  async function togglePin(quest: QuestRow, event?: { stopPropagation?: () => void; preventDefault?: () => void }) {
    event?.stopPropagation?.()
    event?.preventDefault?.()
    if (!quest || pinBusyId != null) return
    pinBusyId = quest.id
    deps.setError('')
    try {
      const patch = quest.pinned
        ? { overlay_hidden: true }
        : quest.overlay_hidden
          ? { overlay_hidden: false }
          : { pinned: true }
      const saved = await updateQuest(quest.id, patch)
      applyQuest(saved, { select: false })
      deps.selectQuest(saved.id, { pushUrl: false })
    } catch (e) {
      deps.setError(e instanceof Error ? e.message : String(e))
    } finally {
      pinBusyId = null
    }
  }

  function kbCursor() {
    const list = deps.getSelected()?.steps ?? []
    if (!list.length) return null
    return list.find((s) => s.id === kbStepId) ?? list.find((s) => !s.done) ?? list[list.length - 1]
  }
  function kbMoveStep(delta: number) {
    const list = deps.getSelected()?.steps ?? []
    const cur = kbCursor()
    if (!cur) return
    if (kbStepId == null) {
      kbStepId = cur.id
      return
    }
    const i = list.findIndex((s) => s.id === cur.id)
    kbStepId = list[Math.min(list.length - 1, Math.max(0, i + delta))].id
  }
  function kbBumpStep(delta: number) {
    const cur = kbCursor()
    const selected = deps.getSelected()
    if (!cur || !selected) return false
    kbStepId = cur.id
    void steps.bump(cur, delta, selected.id)
  }
  function kbToggleStep() {
    const cur = kbCursor()
    const selected = deps.getSelected()
    if (!cur || !selected) return false
    kbStepId = cur.id
    const total = Math.max(0, Number(cur.progress_total) || 0)
    const done = (cur.progress_current ?? 0) >= total
    void steps.set(cur, done ? Math.max(0, total - 1) : total, selected.id)
  }

  return {
    get modalOpen() {
      return modalOpen
    },
    get modalMode() {
      return modalMode
    },
    get modalDefaults() {
      return modalDefaults
    },
    get lineModalOpen() {
      return lineModalOpen
    },
    get lineModalMode() {
      return lineModalMode
    },
    get lineModalTarget() {
      return lineModalTarget
    },
    get stepModalOpen() {
      return stepModalOpen
    },
    get stepModalQuestId() {
      return stepModalQuestId
    },
    get stepModalStep() {
      return stepModalStep
    },
    get kbStepId() {
      return kbStepId
    },
    set kbStepId(id: number | null) {
      kbStepId = id
    },
    get statusBusy() {
      return questActions.busy
    },
    get stepBusyId() {
      return steps.busyId
    },
    get stepEditId() {
      return steps.editId
    },
    get stepEditValue() {
      return steps.editValue
    },
    set stepEditValue(v: string) {
      steps.editValue = v
    },
    menu,
    multi,
    del,
    noteRemote,
    consumeRemoteEdit,
    openCreate,
    openCreateQuestline,
    openEdit,
    closeQuestModal,
    closeStepModal,
    closeLineModal,
    onSaved,
    onDeleted,
    onQuestlineSaved,
    onQuestlineDeleted,
    applyQuest,
    togglePin,
    toggleCompleted: (quest?: QuestRow | null) => questActions.toggleCompleted(quest || deps.getSelected()),
    bumpStep: steps.bump,
    beginEditStep: steps.begin,
    onStepEditKeydown: steps.onKeydown,
    onStepEditBlur: steps.onBlur,
    requestDeleteSelected: del.askQuest,
    kbMoveStep,
    kbBumpStep,
    kbToggleStep,
    questIdOfStep: (stepId: number) => questIdOfStep(deps.getQuests(), stepId),
  }
}
