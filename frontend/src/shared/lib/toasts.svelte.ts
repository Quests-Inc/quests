/** Minimal journal toasts. Import from `.svelte.ts` so the list stays reactive. */
export type ToastKind = 'info' | 'success' | 'error' | 'progress'
export type Toast = { id: string; kind: ToastKind; message: string }

export const toasts: Toast[] = $state([])

const timers = new Map<string, ReturnType<typeof setTimeout>>()
let seq = 0

function clearTimer(id: string) {
  const t = timers.get(id)
  if (t != null) {
    clearTimeout(t)
    timers.delete(id)
  }
}

/** @returns toast id */
export function toast(message: string, opts: { id?: string; kind?: ToastKind; ttl?: number | null } = {}): string {
  const id = opts.id ?? `t${++seq}`
  const kind = opts.kind ?? 'info'
  const ttl = opts.ttl !== undefined ? opts.ttl : kind === 'progress' ? null : kind === 'error' ? 5200 : 2200

  const entry = { id, kind, message: String(message || '').trim() || '…' }
  const idx = toasts.findIndex((t) => t.id === id)
  if (idx >= 0) toasts[idx] = entry
  else toasts.push(entry)

  while (toasts.length > 4) {
    dismissToast(toasts[0].id)
  }

  clearTimer(id)
  if (ttl != null && ttl > 0) {
    timers.set(
      id,
      setTimeout(() => dismissToast(id), ttl)
    )
  }
  return id
}

export function dismissToast(id: string) {
  clearTimer(id)
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}

export function toastProgress(id: string, message: string) {
  return toast(message, { id, kind: 'progress', ttl: null })
}

export function toastDone(id: string, message: string, kind: ToastKind = 'success') {
  return toast(message, {
    id,
    kind,
    ttl: kind === 'error' ? 5200 : 2200,
  })
}

const STATUS_RU: Record<string, string> = {
  active: 'активен',
  expired: 'просрочен',
  frozen: 'заморожен',
  completed: 'выполнен',
  failed: 'провален',
  archived: 'в архиве',
}

export function statusToastLabel(status: string) {
  return STATUS_RU[status] || status
}
