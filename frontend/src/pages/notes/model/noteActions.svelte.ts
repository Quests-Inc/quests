// Быстрые действия над заметкой из дерева и меню: перенос, закрепление, флаги, «устарела», подняться на уровень.
// Каждое сохраняется сразу; если заметка открыта, форма редактора получает значение через `editor.patched`.
import { isUnder, type NoteForm } from '@/entities/note'
import type { NotePayload } from '@/shared/api/knowledge_http.gen.ts'

type Row = { id: number; parent_id?: number | null; pinned?: boolean; outdated?: boolean } & Record<string, unknown>
type FlagField = 'is_readme' | 'is_private' | 'is_category'
const FLAG_FORM = { is_readme: 'isReadme', is_private: 'isPrivate', is_category: 'isCategory' } as const

const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

export function createNoteActions(deps: {
  notes: () => Row[]
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
  const byId = (id: number) => deps.notes().find((n) => n.id === id)

  async function save(id: number, patch: Record<string, unknown>, form: Partial<NoteForm>) {
    deps.editor.error = ''
    try {
      const row = await deps.api.updateNote(id, patch)
      deps.editor.patched(id, row, form)
      deps.onChanged()
    } catch (e) {
      deps.editor.error = messageOf(e)
    }
  }

  async function reparent(id: number, nextParent: number | null) {
    const n = byId(id)
    if (!n) return
    if ((n.parent_id ?? null) === nextParent) return
    if (nextParent != null && (nextParent === id || isUnder(deps.notes(), nextParent, id))) return
    await save(id, { parent_id: nextParent }, { parentId: nextParent == null ? '' : String(nextParent) })
  }

  async function togglePin(id: number) {
    const n = byId(id)
    if (!n) return
    const next = !n.pinned
    await save(id, { pinned: next }, { pinned: next })
  }

  async function toggleFlag(id: number, field: FlagField) {
    const n = byId(id)
    if (!n) return
    const next = !n[field]
    await save(id, { [field]: next }, { [FLAG_FORM[field]]: next })
  }

  async function toggleOutdated(id: number) {
    const n = byId(id)
    if (!n) return
    deps.editor.error = ''
    try {
      await deps.api.setNoteOutdated(id, !n.outdated)
      if (id === deps.selectedId()) deps.editor.detail = await deps.api.getNote(id)
      deps.onChanged()
    } catch (e) {
      deps.editor.error = messageOf(e)
    }
  }

  async function moveUp(id: number) {
    const n = byId(id)
    if (!n?.parent_id) return
    await reparent(id, byId(n.parent_id)?.parent_id ?? null)
  }

  return { reparent, togglePin, toggleFlag, toggleOutdated, moveUp }
}
