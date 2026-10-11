// Переименование строки прямо в дереве: поле ввода на месте названия, Enter — сохранить, Esc — отмена.
import { tick } from 'svelte'

import { toast } from '@/shared/lib/toasts.svelte.ts'

type Row = { id: number; title: string }

export function createNoteRename(deps: {
  noteById: (id: number) => Row | null
  /** Название, с которого начинать правку (с учётом несохранённого черновика). */
  rawTitle: (n: Row) => string
  updateNote: (id: number, patch: { title: string }) => Promise<unknown>
  /** Сервер принял новое название. */
  onSaved: (id: number, title: string, savedRow: any) => void
  setError: (message: string) => void
}) {
  let id = $state<number | null>(null)
  let value = $state('')
  let input = $state<HTMLInputElement | null>(null)

  function cancel() {
    id = null
    value = ''
  }

  function start(noteId: number) {
    const n = deps.noteById(noteId)
    if (!n) return
    id = noteId
    value = deps.rawTitle(n)
    void tick().then(() => {
      input?.focus()
      input?.select()
    })
  }

  async function commit() {
    const noteId = id
    if (noteId == null) return
    const t = value.trim()
    const n = deps.noteById(noteId)
    cancel()
    if (!t || !n || t === n.title) return
    try {
      deps.onSaved(noteId, t, await deps.updateNote(noteId, { title: t }))
    } catch (e) {
      const message = (e as Error)?.message || String(e)
      deps.setError(message)
      toast(message, { kind: 'error' })
    }
  }

  return {
    get id() {
      return id
    },
    get value() {
      return value
    },
    set value(v: string) {
      value = v
    },
    get input() {
      return input
    },
    set input(el: HTMLInputElement | null) {
      input = el
    },
    start,
    cancel,
    commit,
    onKeydown(event: KeyboardEvent) {
      if (event.key === 'Enter') {
        event.preventDefault()
        input?.blur()
      } else if (event.key === 'Escape') {
        event.preventDefault()
        cancel()
      }
      event.stopPropagation()
    },
  }
}
