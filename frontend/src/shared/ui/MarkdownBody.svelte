<script lang="ts">
  import { untrack } from 'svelte'
  import { renderMarkdown } from '@/shared/lib/markdown.ts'
  import { parseRefHref } from '@/shared/lib/refs.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { tailFade } from '@/shared/lib/typewriter.ts'
  import { refPreview } from '@/shared/lib/refPreview.ts'
  import { scriptEmbed } from '@/shared/lib/scriptEmbed.ts'
  import { refIcons } from '@/shared/lib/refIcons.svelte.ts'
  import copySvg from '@/assets/icons/copy.svg?raw'
  import checkSvg from '@/assets/icons/checkmark.svg?raw'

  let {
    source = '',
    class: className = '',
    labels = {},
    onRef,
    typing = false,
  }: {
    source?: string,
    class?: string,
    labels?: Record<string, string>,
    onRef?: (kind: string, id: number) => void,
    typing?: boolean,
  } = $props()

  let html = $derived(renderMarkdown(source, { labels }))
  let root: HTMLDivElement | undefined = $state()

  $effect(() => {
    const el = root
    const _ = html
    if (!el) return
    untrack(() => enhanceCodeBlocks(el))
  })

  function enhanceCodeBlocks(rootEl: HTMLElement) {
    for (const block of rootEl.querySelectorAll('.md-codeblock')) {
      if (block.querySelector(':scope > .md-codeblock__copy')) continue
      const pre = block.querySelector(':scope > pre')
      if (!pre) continue

      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = 'md-codeblock__copy'
      btn.setAttribute('aria-label', 'Копировать')
      btn.title = 'Копировать'
      btn.innerHTML = copySvg

      btn.addEventListener('click', async (event) => {
        event.preventDefault()
        event.stopPropagation()
        const code = pre.querySelector('code')
        const text = code?.textContent ?? pre.textContent ?? ''
        try {
          await copyText(text)
          btn.classList.add('md-codeblock__copy--done')
          btn.innerHTML = checkSvg
          btn.setAttribute('aria-label', 'Скопировано')
          btn.title = 'Скопировано'
          window.setTimeout(() => {
            btn.classList.remove('md-codeblock__copy--done')
            btn.innerHTML = copySvg
            btn.setAttribute('aria-label', 'Копировать')
            btn.title = 'Копировать'
          }, 1400)
        } catch {
          /* ignore */
        }
      })

      block.prepend(btn)
    }
  }

  /**
   * `error` doesn't bubble, so listen in the capture phase: a removed/missing
   * attachment (or a non-image one) becomes a visible placeholder instead of a
   * blank hole.
   */
  function onImgError(event: Event) {
    const img = event.target
    if (!(img instanceof HTMLImageElement) || !img.classList.contains('md-attachment-img')) return
    const ph = document.createElement('span')
    ph.className = 'md-img-missing'
    const m = /\/api\/attachments\/(\d+)\/image/.exec(img.getAttribute('src') || '')
    const alt = img.getAttribute('alt') || ''
    ph.textContent = `▢ вложение недоступно${m ? ` (attachment=${m[1]})` : ''}${alt ? ` — ${alt}` : ''}`
    img.replaceWith(ph)
  }

  function captureImgErrors(node: HTMLElement) {
    node.addEventListener('error', onImgError, true)
    return { destroy: () => node.removeEventListener('error', onImgError, true) }
  }

  function onClick(event: MouseEvent) {
    const a = event.target instanceof Element ? event.target.closest('a') : null
    if (!a) return
    const href = a.getAttribute('href') || ''
    const ref = parseRefHref(href)
    if (!ref) return
    event.preventDefault()
    onRef?.(ref.kind, ref.id)
  }
</script>

{#if html}
  <div
    class={['md', className]}
    bind:this={root}
    onclick={onClick}
    use:captureImgErrors
    use:refIcons
    use:scriptEmbed
    use:refPreview={onRef}
    use:tailFade={typing}
  >{@html html}</div>
{/if}
