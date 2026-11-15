/** Center a floating panel on its anchor, keeping it inside the map or viewport. */
export function centerPanel(node: HTMLElement, options: { center?: number; viewport?: boolean } = {}) {
  let settings = options
  function place() {
    const parent = node.parentElement
    if (!parent) return
    const rect = parent.getBoundingClientRect()
    const width = node.getBoundingClientRect().width
    const min = settings.viewport ? 8 - rect.left : 8
    const max = (settings.viewport ? document.documentElement.clientWidth - rect.left : parent.clientWidth) - width - 8
    const center = settings.center ?? rect.width / 2
    node.style.left = `${Math.max(min, Math.min(center - width / 2, max))}px`
  }
  const observer = new ResizeObserver(place)
  observer.observe(node)
  if (node.parentElement) observer.observe(node.parentElement)
  window.addEventListener('resize', place)
  window.addEventListener('scroll', place, true)
  place()
  return {
    update(next: typeof options) {
      settings = next
      place()
    },
    destroy() {
      observer.disconnect()
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    },
  }
}
