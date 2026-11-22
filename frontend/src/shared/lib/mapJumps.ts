import mapSvg from '@/assets/icons/map.svg?raw'

import { parseRefHref } from './refs.ts'

export type MapJump = {
  keys: string
  jump: (kind: string, id: number) => void
}

/** A map button after each entity link that also sits on this canvas. The link itself is left alone. */
export function mapJumps(node: HTMLElement, spec: MapJump | null) {
  let current = spec
  const apply = () => {
    const keys = current ? new Set(current.keys.split('\n').filter(Boolean)) : null
    for (const a of node.querySelectorAll<HTMLAnchorElement>('a[href]')) {
      const ref = parseRefHref(a.getAttribute('href') || '')
      const on = !!(ref && keys?.has(`${ref.kind}:${ref.id}`) && current)
      const sib = a.nextElementSibling
      const existing = sib instanceof HTMLButtonElement && sib.classList.contains('md-map-jump') ? sib : null
      if (!on || !ref || !current) {
        existing?.remove()
        continue
      }
      const button = existing ?? document.createElement('button')
      const jump = current.jump
      const kind = ref.kind
      const id = ref.id
      if (!existing) {
        button.type = 'button'
        button.className = 'md-map-jump'
        button.title = 'На карте'
        button.setAttribute('aria-label', 'Показать на карте')
        button.innerHTML = mapSvg
        a.after(button)
      }
      button.onclick = (event) => {
        event.preventDefault()
        event.stopPropagation()
        jump(kind, id)
      }
    }
  }
  const mo = new MutationObserver(() => {
    mo.disconnect()
    apply()
    mo.observe(node, { childList: true, subtree: true })
  })
  apply()
  mo.observe(node, { childList: true, subtree: true })
  return {
    update(next: MapJump | null) {
      current = next
      apply()
    },
    destroy() {
      mo.disconnect()
      for (const button of node.querySelectorAll('.md-map-jump')) button.remove()
    },
  }
}
