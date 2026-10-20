// Выделение нескольких вложений на странице «Вложения» (Ctrl/Shift-клик, Ctrl+A) и действия над группой: скачать, скрыть, удалить.
import { ownerKind } from '@/entities/attachment'
import { createMultiSelect } from '@/shared/lib/multiSelect.svelte.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Att = { id: number; owner_id: number; hidden?: boolean } & Record<string, unknown>
type MenuItem = { id: string; label?: string; sep?: boolean; danger?: boolean }

export function createAttachmentGroup(deps: {
  rows: () => Att[]
  selectedId: () => number | null
  download: (a: Att) => void
  setHidden: (kind: string, ownerId: number, id: number, hidden: boolean) => Promise<unknown>
  remove: (kind: string, ownerId: number, id: number) => Promise<unknown>
  onChanged: () => void
  onDeleted: (ids: number[]) => void
}) {
  const multi = createMultiSelect({ current: () => deps.selectedId() })
  /** Ids waiting for the delete confirmation. */
  let ask = $state<number[]>([])
  let busy = $state(false)

  const of = (ids: number[]) => deps.rows().filter((a) => ids.includes(a.id))

  /** Menu of the picked group: what the right click on a picked file offers. */
  function items(ids: number[]): MenuItem[] {
    const list = of(ids)
    const allHidden = list.length > 0 && list.every((a) => a.hidden)
    return [
      { id: 'group-download', label: `Скачать (${list.length})` },
      { id: 'group-hide', label: allHidden ? `Вернуть (${list.length})` : `Скрыть (${list.length})` },
      { id: 'group-clear', label: 'Снять выделение' },
      { id: 'sep-danger', sep: true },
      { id: 'group-delete', label: `Удалить (${list.length})…`, danger: true },
    ]
  }

  async function each(list: Att[], step: (a: Att) => Promise<unknown>) {
    let failed = 0
    for (const a of list) {
      try {
        await step(a)
      } catch {
        failed += 1
      }
    }
    return failed
  }

  async function run(action: string, ids: number[]) {
    const list = of(ids)
    if (!list.length || busy) return
    if (action === 'group-clear') return multi.clear()
    if (action === 'group-download') return void list.forEach((a) => deps.download(a))
    if (action === 'group-delete') {
      ask = list.map((a) => a.id)
      return
    }
    if (action === 'group-hide') {
      busy = true
      const hide = !list.every((a) => a.hidden)
      const failed = await each(list, (a) => deps.setHidden(ownerKind(a), a.owner_id, a.id, hide))
      busy = false
      deps.onChanged()
      if (failed) toast(`Не удалось: ${failed}`, { kind: 'error' })
      else toast(`${hide ? 'Скрыто' : 'Возвращено'}: ${list.length}`, { kind: 'success' })
    }
  }

  async function confirmDelete() {
    const list = of(ask)
    if (!list.length || busy) return
    busy = true
    const done: number[] = []
    const failed = await each(list, async (a) => {
      await deps.remove(ownerKind(a), a.owner_id, a.id)
      done.push(a.id)
    })
    busy = false
    ask = []
    multi.clear()
    deps.onChanged()
    if (done.length) deps.onDeleted(done)
    toast(failed ? `Удалено: ${done.length}, не удалось: ${failed}` : `Удалено вложений: ${done.length}`, {
      kind: failed ? 'error' : 'success',
    })
  }

  return {
    multi,
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
    /** Click on a row: true — it went to the selection (do not open the file). `visible` is the ids in the order on screen. */
    click(e: { ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean }, id: number, visible: number[]) {
      return multi.click(e, id, visible)
    },
    selectAll: (visible: number[]) => multi.selectAll(visible),
  }
}
