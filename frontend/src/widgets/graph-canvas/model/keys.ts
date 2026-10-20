// Клавиши, общие для полей карты и скриптов (по физической клавише, чтобы работали в любой раскладке):
// V — выделение, F — вписать, + / − / 0 — зум, Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y — отмена и повтор, Ctrl+A — выделить всё,
// Delete — убрать выделенное, Esc — по очереди связь, режим выделения, полный экран. Чего у страницы нет — не передаётся, и клавиша не работает.

export type CanvasKeyEvent = {
  key: string
  altKey: boolean
  ctrlKey: boolean
  metaKey: boolean
  code: string
  shiftKey: boolean
  target?: EventTarget | null
  preventDefault: () => void
}

export type CanvasKeyEnv = {
  linking: boolean
  selectMode: boolean
  full: boolean
  menuOpen?: boolean
  cancelLink: () => void
  toggleSelect: () => void
  toggleFull: () => void
  fit: () => void
  /** 1 — ближе, −1 — дальше, 0 — масштаб 1:1. */
  zoom: (dir: 1 | -1 | 0) => void
  undo?: () => void
  redo?: () => void
  selectAll?: () => void
  /** Delete / Backspace над выделенным. */
  remove?: () => void
}

export function dialogOpen() {
  return typeof document !== 'undefined' && Boolean(document.querySelector('.backdrop [role="dialog"]'))
}

export function typing(e: Pick<CanvasKeyEvent, 'target'>) {
  const target = e.target
  if (typeof Element === 'undefined' || !(target instanceof Element)) return false
  return Boolean(target.closest('input, textarea, select, [contenteditable="true"]') || dialogOpen())
}

const ZOOM_IN = new Set(['Equal', 'NumpadAdd'])
const ZOOM_OUT = new Set(['Minus', 'NumpadSubtract'])
const ZOOM_RESET = new Set(['Digit0', 'Numpad0'])

/** True when the key was the canvas's and has been handled. */
export function handleCanvasKey(e: CanvasKeyEvent, env: CanvasKeyEnv): boolean {
  if (e.key === 'Escape') {
    if (env.linking) env.cancelLink()
    else if (env.selectMode) env.toggleSelect()
    else if (env.full && !env.menuOpen && !typing(e)) env.toggleFull()
    else return false
    return true
  }
  if (e.altKey || typing(e)) return false
  const mod = e.ctrlKey || e.metaKey
  let run: (() => void) | undefined
  if (mod) {
    if (e.code === 'KeyZ') run = e.shiftKey ? env.redo : env.undo
    else if (e.code === 'KeyY' && !e.shiftKey) run = env.redo
    else if (e.code === 'KeyA' && !e.shiftKey) run = env.selectAll
  } else if (e.code === 'KeyV' && !e.shiftKey) run = env.toggleSelect
  else if (e.code === 'KeyF' && !e.shiftKey) run = env.fit
  else if (ZOOM_IN.has(e.code)) run = () => env.zoom(1)
  else if (ZOOM_OUT.has(e.code)) run = () => env.zoom(-1)
  else if (ZOOM_RESET.has(e.code)) run = () => env.zoom(0)
  else if ((e.code === 'Delete' || e.code === 'Backspace') && !e.shiftKey) run = env.remove
  if (!run) return false
  e.preventDefault()
  run()
  return true
}
