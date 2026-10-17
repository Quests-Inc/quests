// Несколько выделенных заметок в дереве: Ctrl/Shift-выбор, меню группы и его действия.
// Флаги и перенос идут по одной заметке без перечитывания списка, список обновляется один раз в конце.
import { allHave, moveSearchItems, noteGroupMenuItems, topLevelIds, type MenuItem } from '@/entities/note'
import { createMultiSelect } from '@/shared/lib/multiSelect.svelte.ts'

type Row = {
  id: number
  title?: string
  description?: string
  parent_id?: number | null
  pinned?: boolean
  is_readme?: boolean
  is_private?: boolean
  outdated?: boolean
}
type FlagField = 'is_readme' | 'is_private' | 'is_category'

const rowTitle = (r: Row) => r.title || `note=${r.id}`

export function createNoteGroup(deps: {
  notes: () => Row[]
  selectedId: () => number | null
  /** Несохранённый текст у заметки (открытая правка или черновик). */
  isDirty: (id: number) => boolean
  actions: {
    reparent: (id: number, parent: number | null, silent?: boolean) => Promise<unknown>
    setPin: (id: number, v: boolean, silent?: boolean) => Promise<unknown>
    setFlag: (id: number, f: FlagField, v: boolean, silent?: boolean) => Promise<unknown>
    setOutdated: (id: number, v: boolean, silent?: boolean) => Promise<unknown>
  }
  deleteNote: (id: number) => Promise<unknown>
  /** После удаления: сбросить черновики, снять выбор открытой заметки. */
  afterDelete: (ids: number[]) => void
  onChanged: () => void
  copy: (text: string) => Promise<void>
  downloadMarkdown: (note: { title: string; description: string }) => void
  toast: (message: string, kind: 'success' | 'error') => void
}) {
  const multi = createMultiSelect({ current: () => deps.selectedId() })
  let open = $state(false)
  let x = $state(0)
  let y = $state(0)
  let ids = $state<number[]>([])
  let order: number[] = []
  let busy = $state(false)
  let confirm = $state<{ title: string; message: string; label: string; run: () => Promise<void> } | null>(null)

  const rows = () => ids.map((id) => deps.notes().find((n) => n.id === id)).filter((n): n is Row => !!n)
  const items = $derived<MenuItem[]>(
    open ? noteGroupMenuItems({ rows: rows(), moveSearch: moveSearchItems(ids, deps.notes()) }) : []
  )

  /** Один проход по группе: ошибки считаем, остальных не бросаем; список перечитываем один раз. */
  async function each(list: number[], step: (id: number) => Promise<unknown>, done: (n: number) => string) {
    if (busy) return
    busy = true
    let ok = 0
    let failed = 0
    for (const id of list) {
      try {
        await step(id)
        ok += 1
      } catch {
        failed += 1
      }
    }
    busy = false
    deps.onChanged()
    if (failed) deps.toast(`${done(ok)}, не удалось: ${failed}`, 'error')
    else if (ok) deps.toast(done(ok), 'success')
  }

  async function flag(field: 'pinned' | 'is_readme' | 'is_private' | 'outdated') {
    const list = rows()
    const next = !allHave(list, field)
    const todo = list.filter((r) => !!r[field] !== next).map((r) => r.id)
    const a = deps.actions
    const step = (id: number) =>
      field === 'pinned'
        ? a.setPin(id, next, true)
        : field === 'outdated'
          ? a.setOutdated(id, next, true)
          : a.setFlag(id, field, next, true)
    await each(todo, step, (n) => `${next ? 'Помечено' : 'Снято'}: ${n}`)
  }

  async function move(target: number | null) {
    const top = topLevelIds(deps.notes(), ids).filter((id) => id !== target)
    await each(
      top,
      (id) => deps.actions.reparent(id, target, true),
      (n) => `Перемещено: ${n}`
    )
  }

  function exportMarkdown() {
    const list = rows()
    const rank = (id: number) => (order.includes(id) ? order.indexOf(id) : order.length)
    const parts = [...topLevelIds(deps.notes(), ids)]
      .sort((a, b) => rank(a) - rank(b))
      .map((id) => list.find((r) => r.id === id))
      .filter((r): r is Row => !!r)
      .map((r) =>
        r.description?.trimStart().startsWith('#') ? r.description : `# ${rowTitle(r)}\n\n${r.description || ''}`
      )
    deps.downloadMarkdown({ title: `Заметки (${parts.length})`, description: parts.join('\n\n---\n\n') })
  }

  function askDelete() {
    const list = rows()
    const names = list
      .slice(0, 5)
      .map((r) => `«${rowTitle(r)}»`)
      .join(', ')
    const more = list.length > 5 ? ` и ещё ${list.length - 5}` : ''
    const dirty = list.some((r) => deps.isDirty(r.id))
    confirm = {
      title: `Удалить заметки (${list.length})?`,
      message: `${names}${more}. Дочерние поднимутся в корень, ссылки note=N в других текстах останутся.${dirty ? ' Несохранённые правки в этих заметках пропадут.' : ''}`,
      label: 'Удалить',
      run: async () => {
        const gone: number[] = []
        await each(
          ids,
          async (id) => {
            await deps.deleteNote(id)
            gone.push(id)
          },
          (n) => `Удалено заметок: ${n}`
        )
        deps.afterDelete(gone)
        multi.clear()
      },
    }
  }

  function askUnprivate() {
    confirm = {
      title: 'Снять PRIVATE?',
      message: `Текст ${ids.length} заметок станет виден агентам через MCP.`,
      label: 'Снять',
      run: () => flag('is_private'),
    }
  }

  return {
    multi,
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
    get confirm() {
      return confirm
    },
    get busy() {
      return busy
    },
    /** Меню группы: только если заметка в выделении и там больше одной. */
    openMenu(e: { clientX: number; clientY: number }, id: number): boolean {
      const targets = multi.targets(id)
      if (targets.length < 2) return false
      ids = targets
      x = e.clientX
      y = e.clientY
      open = true
      return true
    },
    close() {
      open = false
    },
    remember(next: number[]) {
      order = next
    },
    async select(action: string) {
      if (action === 'copy-ids') {
        await deps.copy(
          rows()
            .map((r) => `note=${r.id}`)
            .join('\n')
        )
        deps.toast(`Скопировано ссылок: ${ids.length}`, 'success')
      } else if (action === 'pin') await flag('pinned')
      else if (action === 'readme') await flag('is_readme')
      else if (action === 'outdated') await flag('outdated')
      else if (action === 'private') {
        if (allHave(rows(), 'is_private')) askUnprivate()
        else await flag('is_private')
      } else if (action === 'export-md') exportMarkdown()
      else if (action === 'delete') askDelete()
      else if (action.startsWith('move:')) {
        const raw = action.slice('move:'.length)
        await move(raw === 'root' ? null : Number(raw))
      }
    },
    cancelConfirm() {
      if (!busy) confirm = null
    },
    async doConfirm() {
      const c = confirm
      if (!c) return
      await c.run()
      confirm = null
    },
  }
}
