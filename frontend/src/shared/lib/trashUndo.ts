// «Удалено — Отменить»: a toast after a delete that puts the item back from the trash.
import { restoreFromTrash, type TrashItem } from '@/shared/api/client.ts'

import { toast, type ToastAction } from './toasts.svelte.ts'

/** The «Отменить» button for items just moved to the trash. */
export function trashUndo(kind: TrashItem['kind'], ids: number[]): ToastAction {
  return {
    label: 'Отменить',
    run: async () => {
      for (const id of ids) await restoreFromTrash(kind, id)
      toast(
        ids.length > 1
          ? `Восстановлено: ${ids.length}`
          : kind === 'quest'
            ? 'Квест восстановлен'
            : 'Заметка восстановлена',
        {
          kind: 'success',
        }
      )
    },
  }
}

/** The success toast of a delete, with «Отменить». */
export function toastTrashed(kind: TrashItem['kind'], ids: number[], message: string) {
  return toast(message, { kind: 'success', action: trashUndo(kind, ids) })
}
