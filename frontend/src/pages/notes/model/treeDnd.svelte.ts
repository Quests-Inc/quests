// Перетаскивание заметок в дереве: меняет только родителя, порядок (с детьми → закреплённые → sort_order → id) не трогает.
import { isUnder } from '@/entities/note'

type Row = { id: number; parent_id: number | null }

export function createTreeDnd(deps: {
  notes: () => Row[]
  /** Идёт переименование строки — тащить нельзя. */
  renaming: () => boolean
  /** Перенос сохраняется сразу, как выбор родителя в хлебных крошках. */
  move: (id: number, newParentId: number | null) => unknown
}) {
  let dragId = $state<number | null>(null)
  /** String(id) строки под курсором, 'root' — пустой фон списка, null — ничего. */
  let dragOverKey = $state<string | null>(null)

  /** @param targetId null — сделать заметку корневой */
  function canDropOn(targetId: number | null, draggedId = dragId) {
    if (draggedId == null) return false
    if (targetId === draggedId) return false
    // Цель — потомок перетаскиваемой: перенос зациклил бы ветку.
    if (targetId != null && isUnder(deps.notes(), targetId, draggedId)) return false
    return true
  }

  function finish(target: number | null) {
    const draggedId = dragId
    dragId = null
    dragOverKey = null
    if (draggedId == null || !canDropOn(target, draggedId)) return
    deps.move(draggedId, target)
  }

  return {
    get dragOverKey() {
      return dragOverKey
    },
    canDropOn,
    onRowDragStart(event: DragEvent, id: number) {
      if (deps.renaming()) {
        event.preventDefault()
        return
      }
      dragId = id
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = 'move'
        event.dataTransfer.setData('text/plain', String(id))
      }
    },
    onRowDragEnd() {
      dragId = null
      dragOverKey = null
    },
    onRowDragOver(event: DragEvent, id: number) {
      if (!canDropOn(id)) return
      event.preventDefault()
      event.stopPropagation()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
      dragOverKey = String(id)
    },
    onRowDragLeave(_event: DragEvent, id: number) {
      if (dragOverKey === String(id)) dragOverKey = null
    },
    onRowDrop(event: DragEvent, id: number) {
      event.preventDefault()
      event.stopPropagation()
      finish(id)
    },
    onListDragOver(event: DragEvent) {
      if (!canDropOn(null)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
      dragOverKey = 'root'
    },
    onListDragLeave() {
      if (dragOverKey === 'root') dragOverKey = null
    },
    onListDrop(event: DragEvent) {
      event.preventDefault()
      finish(null)
    },
  }
}
