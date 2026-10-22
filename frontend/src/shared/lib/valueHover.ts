import { readValue } from './valueLive.svelte.ts'
import { VALUE_HOVER_MS, parseValueChip, valueDetail, valueTypeLabel } from './valueTip.ts'

export type ValueEditTarget = { kind: string; id: number; name: string }

type HoverOpts = {
  host?: { kind: string; id: number }
  onEdit?: (target: ValueEditTarget) => void
}

/** Delayed hover over `.md-value`, and a right click that asks to edit that value. */
export function valueHover(node: HTMLElement, get: () => HoverOpts, delay = VALUE_HOVER_MS) {
  let timer = 0
  let tip: HTMLDivElement | null = null
  let cursorX = 0

  function hide() {
    window.clearTimeout(timer)
    timer = 0
    tip?.remove()
    tip = null
  }

  function show(chip: HTMLElement) {
    const parsed = parseValueChip(chip.dataset.value || '', get().host)
    if (!parsed) return
    const row = readValue(parsed.host, parsed.name)
    tip?.remove()
    tip = document.createElement('div')
    tip.className = 'md-value-tip'
    tip.setAttribute('role', 'tooltip')
    const type = document.createElement('p')
    type.className = 'md-value-tip__type'
    type.textContent = row ? valueTypeLabel(row.type) : ''
    const name = document.createElement('p')
    name.className = 'md-value-tip__name'
    name.textContent = parsed.name
    const detail = document.createElement('p')
    detail.className = 'md-value-tip__detail'
    const text = row ? valueDetail(row) : chip.textContent || ''
    detail.textContent = text
    if (row?.error) detail.classList.add('md-value-tip__detail--err')
    if (type.textContent) tip.append(type)
    tip.append(name, detail)
    document.body.append(tip)
    const rect = chip.getBoundingClientRect()
    const box = tip.getBoundingClientRect()
    const centre = cursorX || rect.left + rect.width / 2
    const left = Math.min(Math.max(8, centre - box.width / 2), window.innerWidth - box.width - 8)
    const above = rect.top - box.height - 6
    const top = above >= 8 ? above : rect.bottom + 6
    tip.style.left = `${left}px`
    tip.style.top = `${top}px`
  }

  function onOver(event: Event) {
    const chip = event.target instanceof Element ? event.target.closest('.md-value') : null
    if (!(chip instanceof HTMLElement) || !node.contains(chip)) return
    hide()
    if (event instanceof MouseEvent) cursorX = event.clientX
    timer = window.setTimeout(() => show(chip), delay)
  }

  function onMove(event: Event) {
    if (event instanceof MouseEvent) cursorX = event.clientX
  }

  function onOut(event: Event) {
    const chip = event.target instanceof Element ? event.target.closest('.md-value') : null
    if (!(chip instanceof HTMLElement)) return
    const next = event instanceof MouseEvent && event.relatedTarget instanceof Node ? event.relatedTarget : null
    if (next && chip.contains(next)) return
    hide()
  }

  function onMenu(event: Event) {
    if (!(event instanceof MouseEvent)) return
    const chip = event.target instanceof Element ? event.target.closest('.md-value') : null
    if (!(chip instanceof HTMLElement) || !node.contains(chip)) return
    const parsed = parseValueChip(chip.dataset.value || '', get().host)
    if (!parsed) return
    event.preventDefault()
    event.stopPropagation()
    hide()
    get().onEdit?.({ kind: parsed.kind, id: parsed.id, name: parsed.name })
  }

  node.addEventListener('mouseover', onOver)
  node.addEventListener('mousemove', onMove)
  node.addEventListener('mouseout', onOut)
  node.addEventListener('contextmenu', onMenu)
  return {
    destroy() {
      hide()
      node.removeEventListener('mouseover', onOver)
      node.removeEventListener('mousemove', onMove)
      node.removeEventListener('mouseout', onOut)
      node.removeEventListener('contextmenu', onMenu)
    },
  }
}
