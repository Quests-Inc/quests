import { cubicOut } from 'svelte/easing'

export const SLIDE_MS = 110
export const SLIDE_EASE = 'cubic-bezier(0.215, 0.61, 0.355, 1)'

/** Horizontal slide for tab switches: forward enters from the right, back from the left. */
export function slideTab(node: unknown, { dir, out = false }: { dir: number; out?: boolean }) {
  void node
  const sign = out ? -dir : dir
  return {
    duration: SLIDE_MS,
    delay: 0,
    easing: cubicOut,
    css: (t: number) => `transform: translateX(${(1 - t) * sign * 100}%)`,
  }
}
