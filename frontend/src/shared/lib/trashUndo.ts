// «Удалено — Отменить»: a toast after a delete that puts the item back from the trash.
import { restoreFromTrash, type TrashItem } from '@/shared/api/client.ts'

import { toast, type ToastAction } from './toasts.svelte.ts'

const RESTORED: Record<TrashItem['kind'], string> = {
  quest: 'Квест восстановлен',
  note: 'Заметка восстановлена',
  script: 'Скрипт восстановлен',
  hook: 'Хук восстановлен',
  template: 'Шаблон восстановлен',
  questline: 'Квестлайн восстановлен',
}

/** The «Отменить» button for items just moved to the trash; `after` reloads what the page shows. */
export function trashUndo(kind: TrashItem['kind'], ids: number[], after?: () => unknown): ToastAction {
  return {
    label: 'Отменить',
    run: async () => {
      for (const id of ids) await restoreFromTrash(kind, id)
      await after?.()
      toast(ids.length > 1 ? `Восстановлено: ${ids.length}` : RESTORED[kind], { kind: 'success' })
    },
  }
}

/** The success toast of a delete, with «Отменить». */
export function toastTrashed(kind: TrashItem['kind'], ids: number[], message: string, after?: () => unknown) {
  return toast(message, { kind: 'success', action: trashUndo(kind, ids, after) })
}
