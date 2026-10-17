// Быстрые действия над заметкой из дерева и меню: перенос, закрепление, флаги, «устарела», подняться на уровень.
// Каждое сохраняется сразу; если заметка открыта, форма редактора получает значение через `editor.patched`.
import { isUnder, type NoteForm } from '@/entities/note'
import type { NoteListItem, NotePayload } from '@/shared/api/knowledge_http.gen.ts'
type FlagField = 'is_readme' | 'is_private' | 'is_category'
const FLAG_FORM = { is_readme: 'isReadme', is_private: 'isPrivate', is_category: 'isCategory' } as const

const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

export function createNoteActions(deps: {
  notes: () => NoteListItem[]
  selectedId: () => number | null
  editor: {
    error: string
    detail: NotePayload | null
    patched: (id: number, row: NotePayload, patch: Partial<NoteForm>) => void
  }
  onChanged: () => void
  api: {
    updateNote: (id: number, patch: Record<string, unknown>) => Promise<NotePayload>
    setNoteOutdated: (id: number, outdated: boolean) => Promise<unknown>
    getNote: (id: number) => Promise<NotePayload>
  }
}) {
  const byId = (id: number) => deps.notes().find((n) => n.id === id) as NoteListItem | undefined

  /** `silent`: не перечитывать список после каждой (группа делает это один раз в конце); ошибка при этом летит наружу. */
  async function save(id: number, patch: Record<string, unknown>, form: Partial<NoteForm>, silent = false) {
    deps.editor.error = ''
    try {
      const row = await deps.api.updateNote(id, patch)
      deps.editor.patched(id, row, form)
      if (!silent) deps.onChanged()
    } catch (e) {
      deps.editor.error = messageOf(e)
      if (silent) throw e
    }
  }

  async function reparent(id: number, nextParent: number | null, silent = false) {
    const n = byId(id)
    if (!n) return
    if ((n.parent_id ?? null) === nextParent) return
    if (nextParent != null && (nextParent === id || isUnder(deps.notes(), nextParent, id))) return
    await save(id, { parent_id: nextParent }, { parentId: nextParent == null ? '' : String(nextParent) }, silent)
  }

  const setPin = (id: number, next: boolean, silent = false) => save(id, { pinned: next }, { pinned: next }, silent)
  const setFlag = (id: number, field: FlagField, next: boolean, silent = false) =>
    save(id, { [field]: next }, { [FLAG_FORM[field]]: next }, silent)

  async function setOutdated(id: number, next: boolean, silent = false) {
    deps.editor.error = ''
    try {
      await deps.api.setNoteOutdated(id, next)
      if (id === deps.selectedId()) deps.editor.detail = await deps.api.getNote(id)
      if (!silent) deps.onChanged()
    } catch (e) {
      deps.editor.error = messageOf(e)
      if (silent) throw e
    }
  }

  const togglePin = async (id: number) => {
    const n = byId(id)
    if (n) await setPin(id, !n.pinned)
  }
  const toggleFlag = async (id: number, field: FlagField) => {
    const n = byId(id)
    if (n) await setFlag(id, field, !n[field])
  }
  const toggleOutdated = async (id: number) => {
    const n = byId(id)
    if (n) await setOutdated(id, !n.outdated)
  }

  async function moveUp(id: number) {
    const n = byId(id)
    if (!n?.parent_id) return
    await reparent(id, byId(n.parent_id)?.parent_id ?? null)
  }

  return { reparent, togglePin, toggleFlag, toggleOutdated, moveUp, setPin, setFlag, setOutdated }
}
