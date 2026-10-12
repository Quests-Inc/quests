// Правка названия открытой заметки: двойной клик, Enter и клик вне поля сохраняют, Esc отменяет.
// Название правится отдельно от черновика текста, поэтому не делает заметку «несохранённой».
import { tick } from 'svelte'

export function createTitleEdit(deps: {
  selectedId: () => number | null
  title: () => string
  save: (title: string) => Promise<unknown>
  input: () => HTMLInputElement | null
}) {
  let editing = $state(false)
  let draft = $state('')

  /** Другая заметка (или выход): правка названия не переносится. */
  function cancel() {
    editing = false
  }

  async function start() {
    if (editing || deps.selectedId() == null) return
    draft = deps.title()
    editing = true
    await tick()
    deps.input()?.focus()
    deps.input()?.select()
  }

  async function commit() {
    if (!editing || deps.selectedId() == null) return
    const t = draft
    editing = false
    await deps.save(t)
  }

  function onKeydown(event: { key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if (event.key === 'Enter') {
      event.preventDefault()
      deps.input()?.blur()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      editing = false
      deps.input()?.blur()
    }
  }

  return {
    get editing() {
      return editing
    },
    get draft() {
      return draft
    },
    set draft(v: string) {
      draft = v
    },
    start,
    cancel,
    commit,
    onKeydown,
  }
}
