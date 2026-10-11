// Контекстное меню журнала (квестлайн, квест, шаг): где открыто, для чего, и что делает выбранный пункт.
// Сами действия (правка, удаление, статус…) приходят снаружи — здесь только разбор пункта меню.
import { contextMenuItems } from '@/app/model/contextMenu.ts'

type Kind = 'line' | 'quest' | 'step'
type Pt = { preventDefault: () => void; stopPropagation: () => void; clientX: number; clientY: number }
type Quest = { id: number; status?: string }
type Step = { id: number; title?: string; progress_total?: number }
type Line = { id: number; category_id?: number | null }

export function createJournalMenu(deps: {
  quests: () => Quest[]
  questlines: () => Line[]
  findStep: (stepId: number) => { step: Step; questId: number } | null
  selectQuest: (id: number) => void
  selectedQuestId: () => number | null
  copyId: (kind: string, id: number) => void
  openSecrets: (owner: { kind: string; id: number }) => void
  openCreate: (defaults: { questline_id: number; category_id: number | null }) => void
  editQuest: (quest: Quest) => void
  editLine: (line: Line) => void
  askDeleteLine: () => void
  askDeleteQuest: (quest: Quest) => void
  editStep: (questId: number, step: Step) => void
  askDeleteStep: (d: { questId: number; stepId: number; title: string }) => void
  quest: {
    postpone: (q: Quest, minutes: number) => unknown
    patchStatus: (q: Quest, status: string) => unknown
    patchSignificance: (q: Quest, value: string) => unknown
    exportQuest: (q: Quest, mode: 'md' | 'rich' | 'simple') => unknown
  }
  step: {
    bump: (s: Step, delta: number, questId: number) => unknown
    set: (s: Step, value: number, questId: number) => unknown
  }
}) {
  let open = $state(false)
  let x = $state(0)
  let y = $state(0)
  let kind = $state<Kind | null>(null)
  let lineId = $state<number | null>(null)
  let questId = $state<number | null>(null)
  let stepId = $state<number | null>(null)

  const items = $derived(
    contextMenuItems({
      kind,
      lineId,
      quest: deps.quests().find((q) => q.id === questId),
      questId,
      stepId,
      step: stepId != null ? deps.findStep(stepId)?.step : undefined,
    })
  )

  function show(e: Pt) {
    e.preventDefault()
    e.stopPropagation()
    x = e.clientX
    y = e.clientY
    open = true
  }
  function openLine(e: Pt, line: { id: number }) {
    kind = 'line'
    lineId = line.id
    questId = null
    stepId = null
    show(e)
  }
  function openQuest(e: Pt, quest: { id: number }) {
    kind = 'quest'
    questId = quest.id
    lineId = null
    stepId = null
    deps.selectQuest(quest.id)
    show(e)
  }
  function openStep(e: Pt, step: { id: number }) {
    kind = 'step'
    stepId = step.id
    questId = deps.findStep(step.id)?.questId ?? deps.selectedQuestId()
    lineId = null
    show(e)
  }
  function close() {
    open = false
    kind = null
  }

  function selectLine(action: string) {
    if (action === 'copy-id') return deps.copyId('questline', lineId as number)
    if (action === 'secrets') return void (lineId != null && deps.openSecrets({ kind: 'questline', id: lineId }))
    const line = deps.questlines().find((l) => l.id === lineId)
    if (!line && action !== 'delete') return
    if (action === 'add') deps.openCreate({ questline_id: lineId as number, category_id: line?.category_id ?? null })
    else if (action === 'edit' || action === 'icon') deps.editLine(line as Line)
    else if (action === 'delete') deps.askDeleteLine()
  }

  function selectQuestAction(action: string) {
    if (action === 'copy-id') return deps.copyId('quest', questId as number)
    if (action === 'secrets') return void (questId != null && deps.openSecrets({ kind: 'quest', id: questId }))
    const quest = deps.quests().find((q) => q.id === questId)
    if (!quest) return
    if (action === 'edit') return deps.editQuest(quest)
    if (action.startsWith('delay-')) {
      const minutes = Number(action.slice('delay-'.length))
      if (minutes > 0) deps.quest.postpone(quest, minutes)
    } else if (action.startsWith('status:')) {
      const status = action.slice('status:'.length)
      if (status && status !== quest.status) deps.quest.patchStatus(quest, status)
    } else if (action.startsWith('sig:')) deps.quest.patchSignificance(quest, action.slice('sig:'.length))
    else if (action === 'export-md') deps.quest.exportQuest(quest, 'md')
    else if (action === 'export-pdf') deps.quest.exportQuest(quest, 'rich')
    else if (action === 'export-pdf-simple') deps.quest.exportQuest(quest, 'simple')
    else if (action === 'delete') deps.askDeleteQuest(quest)
  }

  async function selectStep(action: string) {
    if (action === 'copy-id') return deps.copyId('step', stepId as number)
    if (action === 'secrets') return void (stepId != null && deps.openSecrets({ kind: 'step', id: stepId }))
    const found = stepId != null ? deps.findStep(stepId) : null
    if (!found) return
    const { step, questId: qid } = found
    if (action === 'inc') await deps.step.bump(step, 1, qid)
    else if (action === 'dec') await deps.step.bump(step, -1, qid)
    else if (action === 'complete') await deps.step.set(step, step.progress_total ?? 0, qid)
    else if (action === 'reset') await deps.step.set(step, 0, qid)
    else if (action === 'edit') deps.editStep(qid, step)
    else if (action === 'delete')
      deps.askDeleteStep({ questId: qid, stepId: step.id, title: step.title || `step=${step.id}` })
  }

  async function select(action: string) {
    if (kind === 'line') selectLine(action)
    else if (kind === 'quest') selectQuestAction(action)
    else if (kind === 'step') await selectStep(action)
  }

  return {
    get open() {
      return open
    },
    get x() {
      return x
    },
    get y() {
      return y
    },
    get items() {
      return items
    },
    get lineId() {
      return lineId
    },
    clearLine() {
      lineId = null
    },
    openLine,
    openQuest,
    openStep,
    close,
    select,
  }
}
