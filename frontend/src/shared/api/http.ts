import { toast } from '@/shared/lib/toasts.svelte.ts'

export const BASE = ''

/** Set by App when a request comes back 401 — flips the UI to the login screen. */
export let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn
}

/** Thrown on 401 so callers can tell "logged out" from a real failure. */
export class UnauthorizedError extends Error {
  constructor() {
    super('Требуется вход')
    this.name = 'UnauthorizedError'
  }
}

export async function request<T = any>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'same-origin',
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-Quests-Source': 'web',
      ...(options.headers || {}),
    },
  })
  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new UnauthorizedError()
  }
  if (res.status === 204) return null

  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      const msg = res.ok ? `Ответ не JSON: ${text.slice(0, 120)}` : `HTTP ${res.status}: ${text.slice(0, 200)}`
      toast(msg, { kind: 'error' })
      throw new Error(msg)
    }
  }

  if (!res.ok) {
    const detail = data?.detail ?? res.statusText
    const msg = typeof detail === 'string' ? detail : JSON.stringify(detail)
    // Belt-and-suspenders: whatever called this may or may not render `msg`
    // itself (some spots do, some — like a modal confirm dialog closing
    // over its own error — quietly don't). A toast means a failure is
    // never *silent*, even where nothing else shows this specific message.
    toast(msg, { kind: 'error' })
    throw new Error(msg)
  }
  return data as T
}

export function quietQs(quiet: boolean) {
  return quiet ? '?quiet=1' : ''
}
