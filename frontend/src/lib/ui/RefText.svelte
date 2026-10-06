<script>
  import DOMPurify from 'dompurify'
  import { tailFade } from '../js/typewriter.js'
  import { renderRefTextHtml, parseRefHref } from '../js/refs.js'

  /** @type {{
   *   source?: string,
   *   class?: string,
   *   labels?: Record<string, string>,
   *   onRef?: (kind: string, id: number) => void,
   *   typing?: boolean,
   * }} */
  let {
    source = '',
    class: className = '',
    labels = {},
    onRef,
    typing = false,
  } = $props()

  const PURIFY = {
    ALLOWED_TAGS: ['a'],
    ALLOWED_ATTR: ['href', 'class'],
  }

  let html = $derived.by(() => {
    const raw = renderRefTextHtml(source, { labels })
    if (!raw) return ''
    if (typeof window === 'undefined') return raw
    return DOMPurify.sanitize(raw, PURIFY)
  })

  function onClick(event) {
    const a = event.target instanceof Element ? event.target.closest('a') : null
    if (!a) return
    const href = a.getAttribute('href') || ''
    const ref = parseRefHref(href)
    if (!ref) return
    event.preventDefault()
    event.stopPropagation()
    onRef?.(ref.kind, ref.id)
  }
</script>

{#if source}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <span class={className} onclick={onClick} use:tailFade={typing}>{@html html}</span>
{/if}
