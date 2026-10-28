// Где в поле ввода стоит каретка: координаты в рамке самого поля (до прокрутки). Меряется копией текста до каретки в скрытом блоке
// с теми же шрифтом, отступами и шириной, поэтому учитывает перенос строк.

export type Caret = { left: number; top: number; height: number }

export function caretXY(ta: HTMLTextAreaElement, pos = ta.selectionStart ?? 0): Caret | null {
  const cs = getComputedStyle(ta)
  const box = document.createElement('div')
  const s = box.style
  s.position = 'absolute'
  s.visibility = 'hidden'
  s.top = '0'
  s.left = '-9999px'
  s.boxSizing = 'border-box'
  s.whiteSpace = 'pre-wrap'
  s.overflowWrap = 'break-word'
  for (const p of [
    'font',
    'letterSpacing',
    'lineHeight',
    'tabSize',
    'textTransform',
    'paddingTop',
    'paddingRight',
    'paddingBottom',
    'paddingLeft',
    'borderTopWidth',
    'borderRightWidth',
    'borderBottomWidth',
    'borderLeftWidth',
    'borderStyle',
  ] as const) {
    s[p] = cs[p]
  }
  // The field's scrollbar narrows its text: the measuring box is as wide as the text area of the field.
  s.width = `${ta.clientWidth + parseFloat(cs.borderLeftWidth) + parseFloat(cs.borderRightWidth)}px`
  box.textContent = ta.value.slice(0, pos)
  const mark = document.createElement('span')
  mark.textContent = '​'
  box.appendChild(mark)
  document.body.appendChild(box)
  try {
    const lh = parseFloat(cs.lineHeight)
    return { left: mark.offsetLeft, top: mark.offsetTop, height: Number.isFinite(lh) ? lh : mark.offsetHeight }
  } finally {
    box.remove()
  }
}

export type Placed = { left: number; top?: number; bottom?: number; width: number }

/**
 * Where a suggestion list goes: under the caret's line, or above it when the room below is too small and above is bigger.
 * `caret` is relative to the wrapper (scroll already taken off); `wrap` is the wrapper's size; `room` is the space the screen
 * gives below and above the caret's line.
 */
export function placePopup(
  caret: Caret,
  wrap: { w: number; h: number },
  popup: { w: number; h: number },
  room: { below: number; above: number }
): Placed {
  const gap = 4
  const width = Math.min(popup.w, Math.max(160, wrap.w - 16))
  const left = Math.max(8, Math.min(caret.left, wrap.w - width - 8))
  const flip = room.below < popup.h + gap && room.above > room.below
  return flip ? { left, width, bottom: wrap.h - caret.top + gap } : { left, width, top: caret.top + caret.height + gap }
}
