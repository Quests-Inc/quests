import { statusWord } from './questStatus.ts'

/** Minimal journal toasts. Import from `.svelte.ts` so the list stays reactive. */
export type ToastKind = 'info' | 'success' | 'error' | 'progress'
/** A button in a toast («Отменить»). It runs once; the toast goes away when it is pressed. */
export type ToastAction = { label: string; run: () => void | Promise<void> }
export type Toast = { id: string; kind: ToastKind; message: string; action?: ToastAction }

/** A toast with a button stays long enough to be pressed. */
const ACTION_TTL = 8000

export const toasts: Toast[] = $state([])

const timers = new Map<string, ReturnType<typeof setTimeout>>()
/** How long each live toast lasts, to start the countdown again after the pointer leaves it. */
const ttls = new Map<string, number>()
let seq = 0

function clearTimer(id: string) {
  const t = timers.get(id)
  if (t != null) {
    clearTimeout(t)
    timers.delete(id)
  }
}

/** @returns toast id */
export function toast(
  message: string,
  opts: { id?: string; kind?: ToastKind; ttl?: number | null; action?: ToastAction } = {}
): string {
  const id = opts.id ?? `t${++seq}`
  const kind = opts.kind ?? 'info'
  const ttl =
    opts.ttl !== undefined
      ? opts.ttl
      : kind === 'progress'
        ? null
        : opts.action
          ? ACTION_TTL
          : kind === 'error'
            ? 5200
            : 2200

  const entry: Toast = {
    id,
    kind,
    message: String(message || '').trim() || '…',
    ...(opts.action ? { action: opts.action } : {}),
  }
  const idx = toasts.findIndex((t) => t.id === id)
  if (idx >= 0) toasts[idx] = entry
  else toasts.push(entry)

  while (toasts.length > 4) {
    dismissToast(toasts[0].id)
  }

  clearTimer(id)
  if (ttl != null && ttl > 0) ttls.set(id, ttl)
  else ttls.delete(id)
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
  ttls.delete(id)
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}

/** The pointer or focus is on the toast: stop its countdown (so a button can be reached). */
export function holdToast(id: string) {
  clearTimer(id)
}

/** The pointer left: count down again from the start. */
export function releaseToast(id: string) {
  const ttl = ttls.get(id)
  if (ttl && toasts.some((t) => t.id === id) && !timers.has(id))
    timers.set(
      id,
      setTimeout(() => dismissToast(id), ttl)
    )
}

/** Press the toast's button: the toast is dismissed first (no double press); a failure comes back as an error toast. */
export async function runToastAction(id: string) {
  const action = toasts.find((t) => t.id === id)?.action
  dismissToast(id)
  if (!action) return
  try {
    await action.run()
  } catch (e) {
    toast((e as Error)?.message || String(e), { kind: 'error' })
  }
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

export function statusToastLabel(status: string) {
  return statusWord(status)
}
