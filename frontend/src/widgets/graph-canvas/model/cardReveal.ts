// Показ markdown и замер карточки в одном проходе, до paint: размер не догоняет текст следующим кадром.
import { flushSync } from 'svelte'

import { afterPaint } from '@/shared/lib/frameTasks.ts'

export function measureCardBox(el: HTMLElement | null, apply: (width: number, height: number) => void) {
  if (!el) return
  const previous = el.style.height
  el.style.height = 'auto'
  const width = el.offsetWidth
  const height = el.offsetHeight
  el.style.height = previous
  flushSync(() => apply(width, height))
}

/** Включает тело карточки. Уже включённое только перезамеряется. Новое ждёт кадр и бюджет. */
export function revealCardBody(opts: {
  defer: boolean
  enabled: boolean
  shown: boolean
  show: (value: boolean) => void
  measure: () => void
  key?: string
}) {
  if (!opts.defer) {
    opts.show(true)
    return
  }
  if (!opts.enabled) {
    opts.show(false)
    return
  }
  if (opts.shown) {
    opts.measure()
    return
  }
  return afterPaint(() => {
    flushSync(() => opts.show(true))
    opts.measure()
  }, opts.key)
}
