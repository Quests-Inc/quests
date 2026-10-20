// Ctrl/Cmd+A в списке (журнал, заметки): выделить все видимые строки. Не трогает «выделить всё» в полях ввода, диалогах и рабочей области справа.

const WORK_AREA =
  'input, textarea, select, [contenteditable="true"], .backdrop, .detail, .notes__page, .graph__stage, .scv'

export function isListSelectAll(e: {
  code: string
  ctrlKey?: boolean
  metaKey?: boolean
  shiftKey?: boolean
  altKey?: boolean
  target?: EventTarget | null
}) {
  if (e.code !== 'KeyA' || !(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey) return false
  const t = e.target
  return !(typeof Element !== 'undefined' && t instanceof Element && t.closest(WORK_AREA))
}
