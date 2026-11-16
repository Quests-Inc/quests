/** A questline page starts with the line, not the quest — land on the quest title. */
export function alignDetailScroll(scroller: HTMLElement, inLine: boolean) {
  const title = inLine ? scroller.querySelector('.detail__quest .detail__title') : null
  if (!(title instanceof HTMLElement)) {
    scroller.scrollTo({ top: 0 })
    return
  }
  const style = getComputedStyle(scroller)
  const pad = parseFloat(style.paddingTop) || 0
  const border = parseFloat(style.borderTopWidth) || 0
  const top =
    title.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop - border - pad
  scroller.scrollTo({ top: Math.max(0, top) })
}
