// Список скриптов: выделение нескольких (Ctrl/Shift-клик, Ctrl+A), копия скрипта, ссылка на него и массовые действия — принадлежность и удаление.
import { PURPOSE_NONE, PURPOSES } from '@/entities/script'
import type { Script } from '@/shared/api/execution.gen.ts'
import { copyText } from '@/shared/lib/clipboard.ts'
import { createMultiSelect } from '@/shared/lib/multiSelect.svelte.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'
import { toastTrashed } from '@/shared/lib/trashUndo.ts'

type Item = { id: string; label?: string; sep?: boolean; danger?: boolean; children?: Item[] }
const messageOf = (e: unknown) => (e instanceof Error ? e.message : String(e))

export function createScriptGroup(deps: {
  scripts: () => Script[]
  selectedId: () => number | null
  api: {
    get: (id: number) => Promise<Script & { graph?: unknown; version: number }>
    create: (body: { title: string; description?: string; purpose?: string; graph: never }) => Promise<{ id: number }>
    update: (id: number, body: { version: number; purpose?: string }) => Promise<unknown>
    remove: (id: number) => Promise<unknown>
  }
  /** The list is read again after a change. */
  reload: () => Promise<unknown>
  /** Scripts are gone: the open one closes. */
  onRemoved: (ids: number[]) => void
}) {
  const multi = createMultiSelect({ current: () => deps.selectedId() })
  /** Ids waiting for the delete confirmation. */
  let ask = $state<number[]>([])
  let busy = $state(false)

  /** A copy beside the original, named «… (копия)»; returns its id. */
  async function duplicate(id: number): Promise<number | null> {
    try {
      const full = await deps.api.get(id)
      const made = await deps.api.create({
        title: `${full.title} (копия)`,
        description: full.description,
        purpose: full.purpose,
        graph: full.graph as never,
      })
      await deps.reload()
      toast('Скрипт продублирован', { kind: 'success' })
      return made.id
    } catch (e) {
      toast(messageOf(e), { kind: 'error' })
      return null
    }
  }

  async function copyRef(id: number) {
    try {
      await copyText(`script=${id}`)
      toast(`Скопировано script=${id}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      toast(messageOf(e), { kind: 'error' })
    }
  }

  /** Menu of the picked group. */
  function items(ids: number[]): Item[] {
    return [
      {
        id: 'group-purpose',
        label: `Принадлежность (${ids.length})`,
        children: [...PURPOSES, PURPOSE_NONE].map((p) => ({ id: `gp:${p.id}`, label: p.title })),
      },
      { id: 'group-clear', label: 'Снять выделение' },
      { id: 'sep-danger', sep: true },
      { id: 'group-delete', label: `Удалить (${ids.length})…`, danger: true },
    ]
  }

  async function setPurpose(ids: number[], purpose: string) {
    busy = true
    let failed = 0
    for (const id of ids) {
      try {
        const cur = await deps.api.get(id)
        if ((cur.purpose ?? '') !== purpose) await deps.api.update(id, { version: cur.version, purpose })
      } catch {
        failed += 1
      }
    }
    busy = false
    await deps.reload()
    toast(failed ? `Не удалось: ${failed}` : `Принадлежность изменена (${ids.length})`, {
      kind: failed ? 'error' : 'success',
    })
  }

  async function confirmDelete() {
    const ids = ask
    if (!ids.length || busy) return
    busy = true
    const done: number[] = []
    let failed = 0
    for (const id of ids) {
      try {
        await deps.api.remove(id)
        done.push(id)
      } catch {
        failed += 1
      }
    }
    busy = false
    ask = []
    multi.clear()
    await deps.reload()
    if (done.length) deps.onRemoved(done)
    if (failed) toast(`В корзине: ${done.length}; сервер отказал в ${failed} (скрипт подключён)`, { kind: 'error' })
    else toastTrashed('script', done, `Скриптов в корзине: ${done.length}`, deps.reload)
  }

  /** Action of the group menu on the picked scripts `ids`. */
  function run(action: string, ids: number[]) {
    if (action === 'group-clear') multi.clear()
    else if (action === 'group-delete') ask = [...ids]
    else if (action.startsWith('gp:')) void setPurpose(ids, action.slice(3))
  }

  return {
    multi,
    duplicate,
    copyRef,
    items,
    run,
    confirmDelete,
    cancelDelete: () => {
      if (!busy) ask = []
    },
    get ask() {
      return ask
    },
    get busy() {
      return busy
    },
    click: (e: { ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean }, id: number, visible: number[]) =>
      multi.click(e, id, visible),
    selectAll: (visible: number[]) => multi.selectAll(visible),
  }
}
