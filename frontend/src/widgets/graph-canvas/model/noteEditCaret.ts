import { caretOffsetFromPoint, mapRenderedOffsetToSource, placeTextareaCaret } from '@/shared/lib/mdCaretMap.ts'

/** Map a click on rendered markdown to an offset in the note source. */
export function sourceCaretFromMarkdownClick(
  root: ParentNode | null | undefined,
  source: string,
  clientX: number,
  clientY: number
): number | null {
  if (!root) return null
  const rendered = root.textContent ?? ''
  return mapRenderedOffsetToSource(source, rendered, caretOffsetFromPoint(root, clientX, clientY))
}

/** Focus a field that may still be hidden while the map finishes measuring. */
export function focusWhenShown(node: HTMLElement, place?: (el: HTMLElement) => void) {
  let tries = 0
  const grab = () => {
    if (place) place(node)
    else node.focus()
    if (document.activeElement !== node && node.isConnected && tries++ < 30) requestAnimationFrame(grab)
  }
  grab()
}

export function focusEditorAt(ta: HTMLTextAreaElement, offset: number | null) {
  requestAnimationFrame(() =>
    focusWhenShown(ta, () => {
      if (offset == null) ta.focus()
      else placeTextareaCaret(ta, offset)
    })
  )
}
