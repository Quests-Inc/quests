// Правки на карте. Сохранённый квест / шаг открывается в обычной форме (изменение уходит в черновик);
// сущность, существующая только в черновике, правится на месте; заметка — прямо на карточке, в markdown.
import { freshTitle, type GraphNodeView, noteEditChanges, questEditFields, stepEditFields } from '@/entities/graph'
import { putGraphDraftOp } from '@/shared/api/graph.ts'
import { getNote } from '@/shared/api/notes.ts'
import { getQuestFull } from '@/shared/api/quests.ts'
import type { DraftOp } from '@/shared/api/store.gen'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type At = { x: number; y: number }
type EditNode = Pick<GraphNodeView, 'key' | 'title'> & Partial<Pick<GraphNodeView, 'type' | 'id' | 'draft' | 'op'>>
type Edge = { kind: string; from: string; to: string }
type EntityModal = {
  open: boolean
  kind: 'note' | 'step' | 'questline'
  at: At
  /** ключ квеста, для которого делается шаг */
  owner: string
  /** ключ карточки, которая должна ссылаться на новую заметку */
  linkFrom: string
  /** сущность, существующая только в черновике, правится на месте */
  edit: { opId: number; payload: any; title: string; description: string; heading: string } | null
}
export type NoteEditing = {
  key: string
  title: string
  text: string
  orig: { title: string; description: string }
  opId: number | null
  payload?: any
  fresh?: boolean
}

const PLAIN: Pick<EntityModal, 'owner' | 'linkFrom' | 'edit'> = { owner: '', linkFrom: '', edit: null }

export function createGraphEdits(deps: {
  ops: () => DraftOp[]
  addOp: (kind: string, payload: Record<string, unknown>) => Promise<{ id: number } | null | unknown>
  reloadDraft: () => Promise<void>
  edges: () => Edge[]
  noteTitles: () => (string | undefined)[]
}) {
  let entityModal = $state<EntityModal>({ open: false, kind: 'note', at: { x: 0, y: 0 }, ...PLAIN })
  let editQuest = $state<{ open: boolean; quest: any | null }>({ open: false, quest: null })
  let editStep = $state<{ open: boolean; questId: number | null; step: any | null }>({
    open: false,
    questId: null,
    step: null,
  })
  let editing = $state<NoteEditing | null>(null)

  const opAddedId = (op: unknown) => (op as { id?: number } | null)?.id

  function openEntity(kind: EntityModal['kind'], at: At, extra: Partial<Pick<EntityModal, 'owner'>> = {}) {
    entityModal = { open: true, kind, at, ...PLAIN, ...extra }
  }

  async function startEdit(n: EditNode, at: At) {
    if (n.type === 'note') return startEditNote(n)
    if (n.draft) {
      const op = deps.ops().find((o) => o.id === n.op)
      if (!op) return
      const f = (op.payload as any)?.fields ?? {}
      entityModal = {
        open: true,
        kind: 'step',
        at,
        owner: '',
        linkFrom: '',
        edit: {
          opId: op.id,
          payload: op.payload,
          title: f.title ?? n.title,
          description: f.description ?? '',
          heading:
            n.type === 'quest' ? 'Изменить квест' : n.type === 'questline' ? 'Изменить квестлайн' : 'Изменить шаг',
        },
      }
      return
    }
    try {
      if (n.type === 'quest') {
        editQuest = { open: true, quest: await getQuestFull(Number(n.id)) }
      } else if (n.type === 'step') {
        const owner = deps.edges().find((e) => e.kind === 'structure' && e.to === n.key && e.from.startsWith('quest:'))
        if (!owner) return
        const qid = Number(owner.from.slice('quest:'.length))
        const full = await getQuestFull(qid)
        const step = (full.steps ?? []).find((x: { id: number }) => x.id === n.id)
        if (step) editStep = { open: true, questId: qid, step }
      }
    } catch {
      /* toast already shown */
    }
  }

  async function draftEditQuest(payload: Record<string, any>): Promise<void> {
    const orig = editQuest.quest
    if (!orig) return
    const fields = questEditFields(orig, payload)
    if (!Object.keys(fields).length) return void toast('Ничего не изменилось', { kind: 'info' })
    await deps.addOp('edit_quest', { id: `quest:${orig.id}`, fields })
  }

  async function draftEditStep(payload: Record<string, any>): Promise<void> {
    const orig = editStep.step
    if (!orig) return
    const fields = stepEditFields(orig, payload)
    if (!Object.keys(fields).length) return void toast('Ничего не изменилось', { kind: 'info' })
    await deps.addOp('edit_step', { id: `step:${orig.id}`, fields })
  }

  async function putFields(opId: number, payload: any, fields: Record<string, unknown>) {
    try {
      await putGraphDraftOp(opId, { ...payload, fields: { ...(payload.fields ?? {}), ...fields } })
      await deps.reloadDraft()
    } catch {
      /* toast already shown */
    }
  }

  /** Маленькая форма «название + описание» (новая заметка / шаг / квестлайн или правка на месте). */
  async function submitEntity(fields: Record<string, unknown>) {
    const m = entityModal
    if (m.edit) return putFields(m.edit.opId, m.edit.payload, fields)
    if (m.kind === 'step') {
      await deps.addOp('create_step', { fields, ...(m.owner ? { quest: m.owner } : {}), pos: m.at })
      return
    }
    if (m.kind === 'questline') {
      await deps.addOp('create_questline', { fields, pos: m.at })
      return
    }
    const id = opAddedId(await deps.addOp('create_note', { fields, pos: m.at }))
    if (id != null && m.linkFrom) await deps.addOp('add_ref', { from: m.linkFrom, to: `note:t${id}` })
  }

  /** Новая заметка появляется под курсором с выделенным заголовком — сразу печатать поверх. */
  async function createNoteHere(at: At, linkFrom: string) {
    const title = freshTitle([...deps.noteTitles(), ...deps.ops().map((o) => (o.payload as any)?.fields?.title)])
    const id = opAddedId(await deps.addOp('create_note', { fields: { title, description: '' }, pos: at }))
    if (id == null) return
    if (linkFrom) await deps.addOp('add_ref', { from: linkFrom, to: `note:t${id}` })
    await startEditNote({ key: `note:t${id}`, draft: true, op: id, title }, true)
  }

  /** Заметка правится на карточке: полный текст берётся с сервера (на карте виден только его начало). */
  async function startEditNote(n: EditNode, fresh = false) {
    if (editing?.key === n.key) return
    try {
      if (n.draft) {
        const op = deps.ops().find((o) => o.id === n.op)
        if (!op) return
        const f = (op.payload as any)?.fields ?? {}
        editing = {
          key: n.key,
          title: f.title ?? n.title,
          text: f.description ?? '',
          orig: { title: f.title ?? '', description: f.description ?? '' },
          opId: op.id,
          payload: op.payload,
          fresh,
        }
      } else {
        const full = await getNote(Number(n.id))
        editing = {
          key: n.key,
          title: full.title ?? n.title ?? '',
          text: full.description ?? '',
          orig: { title: full.title ?? '', description: full.description ?? '' },
          opId: null,
        }
      }
    } catch {
      /* toast already shown */
    }
  }

  async function finishEdit(saveIt: boolean) {
    const e = editing
    editing = null
    if (!saveIt || !e) return
    const changed = noteEditChanges(e)
    if (!Object.keys(changed).length) return
    if (e.opId != null) return putFields(e.opId, e.payload, changed)
    await deps.addOp('edit_note', { id: e.key, fields: changed })
  }

  return {
    get entityModal() {
      return entityModal
    },
    get editQuest() {
      return editQuest
    },
    get editStep() {
      return editStep
    },
    get editing() {
      return editing
    },
    set editing(v: NoteEditing | null) {
      editing = v
    },
    closeEntity: () => (entityModal = { ...entityModal, open: false }),
    closeEditQuest: () => (editQuest = { ...editQuest, open: false }),
    closeEditStep: () => (editStep = { ...editStep, open: false }),
    openEntity,
    startEdit,
    draftEditQuest,
    draftEditStep,
    submitEntity,
    createNoteHere,
    startEditNote,
    finishEdit,
  }
}
