// Иконка сущности рядом с резолвнутым именем ссылки kind=id. Одно место для markdown (MarkdownBody) и коротких подписей (RefText).
// Правило: иконка по умолчанию (`document` без своей картинки) не показывается, чтобы не шуметь; всё остальное — да.

import { parseRefHref } from './refs.ts'

export type RefIcon = { icon?: string | null; url?: string | null }

let builtin: Record<string, string> | null = null
/** URL встроенной svg-иконки; вне vite (bun-тесты) glob недоступен — тогда иконок просто нет. */
function builtinUrl(name: string) {
  if (!builtin) {
    try {
      builtin = import.meta.glob('@/assets/icons/*.svg', { query: '?url', import: 'default', eager: true }) as Record<
        string,
        string
      >
    } catch {
      builtin = {}
    }
  }
  return Object.entries(builtin).find(([p]) => p.endsWith(`/${name}.svg`))?.[1] ?? null
}

const DEFAULT_ICON = 'document'

/** Иконки по ключу `kind:id`; App кладёт сюда результат buildRefIcons. */
export const refIconStore = $state<{ map: Record<string, RefIcon> }>({ map: {} })

function maskUrl(i: RefIcon | undefined) {
  if (!i) return null
  if (i.url) return i.url
  if (!i.icon || i.icon === DEFAULT_ICON) return null
  return builtinUrl(i.icon)
}

/** Svelte action: ставит `<span class="md-ref__ico">` в начало каждой ссылки-рефа внутри узла. Повторно безопасна. */
export function refIcons(node: HTMLElement) {
  const apply = () => {
    const map = refIconStore.map
    for (const a of node.querySelectorAll<HTMLAnchorElement>('a[href]')) {
      const ref = parseRefHref(a.getAttribute('href') || '')
      const url = ref ? maskUrl(map[`${ref.kind}:${ref.id}`]) : null
      const old = a.querySelector(':scope > .md-ref__ico')
      if (!url) {
        old?.remove()
        continue
      }
      const ico = (old as HTMLElement) ?? document.createElement('span')
      ico.className = 'md-ref__ico'
      ico.setAttribute('aria-hidden', 'true')
      ico.style.setProperty('--ref-ico', `url("${url}")`)
      if (!old) a.prepend(ico)
    }
  }
  const mo = new MutationObserver(() => {
    mo.disconnect()
    apply()
    mo.observe(node, { childList: true, subtree: true })
  })
  apply()
  mo.observe(node, { childList: true, subtree: true })
  $effect(() => {
    void refIconStore.map
    apply()
  })
  return { destroy: () => mo.disconnect() }
}
